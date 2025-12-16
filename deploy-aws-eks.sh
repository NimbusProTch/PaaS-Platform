#!/bin/bash

# AWS EKS Deployment Script
# This script deploys the complete InfraForge platform on AWS EKS

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
ENVIRONMENT=${ENVIRONMENT:-dev}
AWS_REGION=${AWS_REGION:-eu-west-1}
PROJECT_NAME=${PROJECT_NAME:-infraforge}
CLUSTER_NAME="${PROJECT_NAME}-${ENVIRONMENT}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}   InfraForge AWS EKS Deployment${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Environment: ${GREEN}${ENVIRONMENT}${NC}"
echo -e "AWS Region: ${GREEN}${AWS_REGION}${NC}"
echo -e "Cluster Name: ${GREEN}${CLUSTER_NAME}${NC}"
echo

# Function to check prerequisites
check_prerequisites() {
    echo -e "${BLUE}Checking prerequisites...${NC}"

    # Check AWS CLI
    if ! command -v aws &> /dev/null; then
        echo -e "${RED}✗ AWS CLI is not installed${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ AWS CLI found${NC}"

    # Check AWS credentials
    if ! aws sts get-caller-identity &> /dev/null; then
        echo -e "${RED}✗ AWS credentials not configured${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ AWS credentials configured${NC}"

    # Check OpenTofu/Terraform
    if command -v tofu &> /dev/null; then
        TF_CMD="tofu"
        echo -e "${GREEN}✓ OpenTofu found${NC}"
    elif command -v terraform &> /dev/null; then
        TF_CMD="terraform"
        echo -e "${GREEN}✓ Terraform found${NC}"
    else
        echo -e "${RED}✗ Neither OpenTofu nor Terraform is installed${NC}"
        exit 1
    fi

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        echo -e "${RED}✗ kubectl is not installed${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ kubectl found${NC}"

    # Check helm
    if ! command -v helm &> /dev/null; then
        echo -e "${RED}✗ Helm is not installed${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Helm found${NC}"

    echo
}

# Function to create terraform.tfvars if it doesn't exist
create_tfvars() {
    local TFVARS_FILE="infrastructure/aws/terraform.tfvars"

    if [ ! -f "$TFVARS_FILE" ]; then
        echo -e "${BLUE}Creating terraform.tfvars...${NC}"

        read -p "Enter your email address: " OWNER_EMAIL
        read -p "Enter domain name (optional, press Enter to skip): " DOMAIN_NAME

        cat > "$TFVARS_FILE" <<EOF
# InfraForge Platform Configuration
project_name = "${PROJECT_NAME}"
environment  = "${ENVIRONMENT}"
tenant       = "platform"
aws_region   = "${AWS_REGION}"
owner_email  = "${OWNER_EMAIL}"

# VPC Configuration
vpc_cidr           = "10.0.0.0/16"
enable_nat_gateway = true
single_nat_gateway = $([ "$ENVIRONMENT" = "prod" ] && echo "false" || echo "true")

# EKS Configuration
cluster_version = "1.28"

# Node Groups
node_groups = {
  general = {
    desired_size   = $([ "$ENVIRONMENT" = "prod" ] && echo "3" || echo "2")
    min_size      = $([ "$ENVIRONMENT" = "prod" ] && echo "3" || echo "1")
    max_size      = 10
    instance_types = ["t3.large"]
    capacity_type  = "$([ "$ENVIRONMENT" = "prod" ] && echo "ON_DEMAND" || echo "SPOT")"
    disk_size     = 100
    labels = {
      workload = "general"
    }
    taints = []
  }
}

# Enable Production Components
enable_aws_load_balancer_controller = true
enable_external_dns                 = $([ -n "$DOMAIN_NAME" ] && echo "true" || echo "false")
enable_cert_manager                 = true
enable_metrics_server               = true
enable_cluster_autoscaler          = true
enable_ebs_csi_driver              = true
enable_kong                        = true
enable_prometheus                  = $([ "$ENVIRONMENT" = "prod" ] && echo "true" || echo "true")
enable_grafana                     = $([ "$ENVIRONMENT" = "prod" ] && echo "true" || echo "true")

# Optional Components
enable_loki   = $([ "$ENVIRONMENT" = "prod" ] && echo "true" || echo "false")
enable_tempo  = $([ "$ENVIRONMENT" = "prod" ] && echo "true" || echo "false")
enable_velero = $([ "$ENVIRONMENT" = "prod" ] && echo "true" || echo "false")

# Domain Configuration
domain_name         = "${DOMAIN_NAME}"
create_route53_zone = false

# Backstage (to be configured later)
enable_backstage = false
EOF
        echo -e "${GREEN}✓ terraform.tfvars created${NC}"
    else
        echo -e "${YELLOW}⚠ terraform.tfvars already exists, using existing configuration${NC}"
    fi
    echo
}

# Function to deploy infrastructure
deploy_infrastructure() {
    echo -e "${BLUE}Deploying AWS infrastructure...${NC}"
    cd infrastructure/aws

    # Initialize Terraform
    echo -e "${BLUE}Initializing Terraform...${NC}"
    $TF_CMD init -upgrade

    # Plan deployment
    echo -e "${BLUE}Planning deployment...${NC}"
    $TF_CMD plan -out=tfplan

    # Ask for confirmation
    echo
    read -p "Do you want to apply this plan? (yes/no): " CONFIRM
    if [ "$CONFIRM" != "yes" ]; then
        echo -e "${YELLOW}Deployment cancelled${NC}"
        exit 0
    fi

    # Apply deployment
    echo -e "${BLUE}Applying infrastructure...${NC}"
    $TF_CMD apply tfplan

    cd ../..
    echo -e "${GREEN}✓ Infrastructure deployed${NC}"
    echo
}

# Function to update kubeconfig
update_kubeconfig() {
    echo -e "${BLUE}Updating kubeconfig...${NC}"
    aws eks update-kubeconfig --region ${AWS_REGION} --name ${CLUSTER_NAME}
    echo -e "${GREEN}✓ kubeconfig updated${NC}"
    echo
}

# Function to verify deployment
verify_deployment() {
    echo -e "${BLUE}Verifying deployment...${NC}"

    # Check nodes
    echo "Checking nodes..."
    kubectl get nodes
    echo

    # Check pods
    echo "Checking system pods..."
    kubectl get pods -n kube-system
    echo

    # Check Kong
    echo "Checking Kong API Gateway..."
    kubectl get svc -n kong kong-proxy 2>/dev/null || echo "Kong not yet ready"
    echo

    # Check storage classes
    echo "Checking storage classes..."
    kubectl get storageclass
    echo

    # Get Load Balancer URL
    echo -e "${BLUE}Getting Kong Load Balancer URL...${NC}"
    KONG_URL=$(kubectl get svc -n kong kong-proxy -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "pending")
    if [ "$KONG_URL" != "pending" ] && [ -n "$KONG_URL" ]; then
        echo -e "${GREEN}Kong URL: http://${KONG_URL}${NC}"
    else
        echo -e "${YELLOW}Kong Load Balancer is still provisioning...${NC}"
    fi
    echo

    # Get Grafana info if enabled
    if kubectl get svc -n monitoring kube-prometheus-stack-grafana &>/dev/null; then
        echo -e "${BLUE}Getting Grafana access info...${NC}"
        GRAFANA_URL=$(kubectl get svc -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "")
        if [ -n "$GRAFANA_URL" ]; then
            GRAFANA_PASSWORD=$(kubectl get secret -n monitoring kube-prometheus-stack-grafana -o jsonpath="{.data.admin-password}" | base64 -d)
            echo -e "${GREEN}Grafana URL: http://${GRAFANA_URL}${NC}"
            echo -e "${GREEN}Grafana Username: admin${NC}"
            echo -e "${GREEN}Grafana Password: ${GRAFANA_PASSWORD}${NC}"
        fi
    fi
    echo
}

# Function to deploy operators
deploy_operators() {
    echo -e "${BLUE}Do you want to deploy platform operators (PostgreSQL, Redis, Vault, etc.)?${NC}"
    read -p "Deploy operators? (yes/no): " DEPLOY_OPS

    if [ "$DEPLOY_OPS" = "yes" ]; then
        echo -e "${BLUE}Deploying ArgoCD...${NC}"

        # Create namespace
        kubectl create namespace infraforge-argocd --dry-run=client -o yaml | kubectl apply -f -

        # Install ArgoCD
        kubectl apply -n infraforge-argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

        echo -e "${YELLOW}Waiting for ArgoCD to be ready...${NC}"
        kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n infraforge-argocd

        # Deploy operators ApplicationSet
        echo -e "${BLUE}Deploying operators...${NC}"
        kubectl apply -f manifests/platform-cluster/operators/applicationset.yaml

        echo -e "${GREEN}✓ Operators deployed via ArgoCD${NC}"
        echo

        # Get ArgoCD password
        ARGOCD_PASSWORD=$(kubectl -n infraforge-argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
        echo -e "${GREEN}ArgoCD admin password: ${ARGOCD_PASSWORD}${NC}"
        echo -e "${YELLOW}Access ArgoCD by port-forwarding: kubectl port-forward svc/argocd-server -n infraforge-argocd 8080:443${NC}"
    fi
}

# Main execution
main() {
    echo -e "${BLUE}Starting AWS EKS deployment...${NC}"
    echo

    # Check prerequisites
    check_prerequisites

    # Create terraform.tfvars
    create_tfvars

    # Deploy infrastructure
    deploy_infrastructure

    # Update kubeconfig
    update_kubeconfig

    # Verify deployment
    verify_deployment

    # Deploy operators
    deploy_operators

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}   Deployment Complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo
    echo "Next steps:"
    echo "1. Configure DNS records to point to Kong Load Balancer"
    echo "2. Deploy your applications using kubectl or ArgoCD"
    echo "3. Access monitoring dashboards (Grafana)"
    echo "4. Configure Backstage developer portal (optional)"
    echo
    echo "Useful commands:"
    echo "  kubectl get nodes                    # Check cluster nodes"
    echo "  kubectl get pods -A                  # Check all pods"
    echo "  kubectl get gateway -A               # Check Gateway API resources"
    echo "  kubectl logs -n kong deployment/kong # Check Kong logs"
    echo
}

# Run main function
main "$@"