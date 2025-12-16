#!/bin/bash

# Deploy Platform to AWS EKS Script
# This script will setup the platform on AWS EKS

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
ENVIRONMENT=${1:-dev}
AWS_REGION=${AWS_REGION:-eu-west-1}
CLUSTER_NAME="infraforge-${ENVIRONMENT}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}   InfraForge Platform AWS Deployment${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Environment: ${GREEN}${ENVIRONMENT}${NC}"
echo -e "Region: ${GREEN}${AWS_REGION}${NC}"
echo -e "Cluster: ${GREEN}${CLUSTER_NAME}${NC}"
echo

# Check prerequisites
echo -e "${BLUE}Step 1: Checking prerequisites...${NC}"

# Check AWS CLI
if ! command -v aws &> /dev/null; then
    echo -e "${RED}✗ AWS CLI not found. Please install it first.${NC}"
    exit 1
fi

# Check OpenTofu/Terraform
if command -v tofu &> /dev/null; then
    TF_CMD="tofu"
elif command -v terraform &> /dev/null; then
    TF_CMD="terraform"
else
    echo -e "${RED}✗ OpenTofu or Terraform not found. Please install one of them.${NC}"
    exit 1
fi

# Check kubectl
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}✗ kubectl not found. Please install it first.${NC}"
    exit 1
fi

# Check helm
if ! command -v helm &> /dev/null; then
    echo -e "${RED}✗ Helm not found. Please install it first.${NC}"
    exit 1
fi

# Check AWS credentials
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}✗ AWS credentials not configured. Please run 'aws configure'.${NC}"
    exit 1
fi

echo -e "${GREEN}✓ All prerequisites met${NC}"

# Step 2: Create AWS Infrastructure
echo -e "${BLUE}Step 2: Creating AWS infrastructure...${NC}"

cd infrastructure/aws

# Initialize Terraform/OpenTofu
echo -e "${YELLOW}Initializing Terraform/OpenTofu...${NC}"
${TF_CMD} init

# Plan infrastructure
echo -e "${YELLOW}Planning infrastructure...${NC}"
${TF_CMD} plan -var-file=environments/${ENVIRONMENT}.tfvars -out=tfplan

# Confirm deployment
echo -e "${YELLOW}The following resources will be created:${NC}"
echo "- VPC with public/private subnets across 3 AZs"
echo "- EKS cluster with managed node groups"
echo "- IAM roles for service accounts (IRSA)"
echo "- Security groups and networking"
echo

read -p "Do you want to continue? (yes/no): " confirm
if [ "$confirm" != "yes" ]; then
    echo -e "${RED}Deployment cancelled.${NC}"
    exit 1
fi

# Apply infrastructure
echo -e "${YELLOW}Creating infrastructure (this will take 10-15 minutes)...${NC}"
${TF_CMD} apply tfplan

# Get outputs
CLUSTER_NAME=$(${TF_CMD} output -raw cluster_name)
AWS_REGION=$(${TF_CMD} output -raw region)

echo -e "${GREEN}✓ AWS infrastructure created${NC}"

# Step 3: Update kubeconfig
echo -e "${BLUE}Step 3: Updating kubeconfig...${NC}"
aws eks update-kubeconfig --name ${CLUSTER_NAME} --region ${AWS_REGION}
echo -e "${GREEN}✓ Kubeconfig updated${NC}"

# Verify cluster connection
echo -e "${YELLOW}Verifying cluster connection...${NC}"
kubectl get nodes
echo -e "${GREEN}✓ Connected to cluster${NC}"

cd ../..

# Step 4: Install Operators
echo -e "${BLUE}Step 4: Installing operators...${NC}"

# CloudNativePG
echo -e "${YELLOW}Installing CloudNativePG...${NC}"
kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.23/releases/cnpg-1.23.0.yaml

# RabbitMQ Operator
echo -e "${YELLOW}Installing RabbitMQ Operator...${NC}"
kubectl apply -f https://github.com/rabbitmq/cluster-operator/releases/latest/download/cluster-operator.yml

# Redis Operator
echo -e "${YELLOW}Installing Redis Operator...${NC}"
helm repo add ot-helm https://ot-container-kit.github.io/helm-charts/ 2>/dev/null || true
helm repo update
helm install redis-operator ot-helm/redis-operator --create-namespace --namespace redis-operator-system

# MinIO Operator
echo -e "${YELLOW}Installing MinIO Operator...${NC}"
kubectl apply -k 'https://github.com/minio/operator/resources?ref=v6.0.0'

echo -e "${GREEN}✓ Operators installed${NC}"

# Step 5: Install ArgoCD
echo -e "${BLUE}Step 5: Installing ArgoCD...${NC}"
kubectl create namespace infraforge-argocd || true
kubectl apply -n infraforge-argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for ArgoCD to be ready
echo -e "${YELLOW}Waiting for ArgoCD to be ready...${NC}"
kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n infraforge-argocd
echo -e "${GREEN}✓ ArgoCD installed${NC}"

# Step 6: Configure ArgoCD
echo -e "${BLUE}Step 6: Configuring ArgoCD...${NC}"

# Create AppProjects
kubectl apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infraforge-${ENVIRONMENT}
  namespace: infraforge-argocd
spec:
  description: ${ENVIRONMENT} environment for InfraForge platform
  sourceRepos:
  - 'https://github.com/NimbusProTch/PaaS-Platform.git'
  destinations:
  - namespace: '*'
    server: 'https://kubernetes.default.svc'
  clusterResourceWhitelist:
  - group: '*'
    kind: '*'
  namespaceResourceWhitelist:
  - group: '*'
    kind: '*'
EOF

# Grant permissions to ArgoCD
kubectl create clusterrolebinding argocd-application-controller-cluster-admin \
  --clusterrole=cluster-admin \
  --serviceaccount=infraforge-argocd:argocd-application-controller 2>/dev/null || true

echo -e "${GREEN}✓ ArgoCD configured${NC}"

# Step 7: Create namespaces
echo -e "${BLUE}Step 7: Creating namespaces...${NC}"
kubectl create namespace demo-${ENVIRONMENT} || true
echo -e "${GREEN}✓ Namespaces created${NC}"

# Step 8: Deploy ApplicationSets
echo -e "${BLUE}Step 8: Deploying ApplicationSets...${NC}"
kubectl apply -f manifests/platform-cluster/appsets/${ENVIRONMENT}/operator.yaml

# Restart ApplicationSet controller
kubectl rollout restart deployment/argocd-applicationset-controller -n infraforge-argocd

echo -e "${GREEN}✓ ApplicationSets deployed${NC}"

# Step 9: Wait for applications to sync
echo -e "${BLUE}Step 9: Syncing applications...${NC}"
sleep 30

# Force refresh all apps
kubectl get applications -n infraforge-argocd -o name 2>/dev/null | while read app; do
  kubectl -n infraforge-argocd patch $app --type merge -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}' 2>/dev/null || true
done

# Enable auto-sync
kubectl get applications -n infraforge-argocd -o name 2>/dev/null | while read app; do
  kubectl -n infraforge-argocd patch $app --type merge -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}' 2>/dev/null || true
done

echo -e "${GREEN}✓ Applications syncing${NC}"

# Step 10: Install AWS Load Balancer Controller (optional but recommended)
echo -e "${BLUE}Step 10: Installing AWS Load Balancer Controller...${NC}"

# Get IAM role ARN from Terraform output
cd infrastructure/aws
LB_ROLE_ARN=$(${TF_CMD} output -raw aws_load_balancer_controller_role_arn 2>/dev/null || echo "")
cd ../..

if [ ! -z "$LB_ROLE_ARN" ]; then
  helm repo add eks https://aws.github.io/eks-charts 2>/dev/null || true
  helm repo update
  helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
    -n kube-system \
    --set clusterName=${CLUSTER_NAME} \
    --set serviceAccount.create=true \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=${LB_ROLE_ARN} \
    --set region=${AWS_REGION} \
    --set vpcId=$(cd infrastructure/aws && ${TF_CMD} output -raw vpc_id && cd ../..) || true
  echo -e "${GREEN}✓ AWS Load Balancer Controller installed${NC}"
else
  echo -e "${YELLOW}⚠ AWS Load Balancer Controller skipped (IAM role not found)${NC}"
fi

# Final status check
echo -e "${BLUE}Step 11: Checking deployment status...${NC}"
echo
sleep 60

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}        DEPLOYMENT STATUS${NC}"
echo -e "${BLUE}========================================${NC}"
echo

echo -e "${YELLOW}Cluster Nodes:${NC}"
kubectl get nodes

echo
echo -e "${YELLOW}Operators:${NC}"
kubectl get pods -A | grep -E "postgres|rabbit|redis|minio|vault" | grep -i operator

echo
echo -e "${YELLOW}ArgoCD Applications:${NC}"
kubectl get applications -n infraforge-argocd 2>/dev/null || echo "No applications yet"

echo
echo -e "${YELLOW}Platform Services (${ENVIRONMENT}):${NC}"
kubectl get pods -n demo-${ENVIRONMENT} 2>/dev/null || echo "Services are being deployed..."

echo
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}    AWS Deployment Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo

echo -e "${BLUE}Access Information:${NC}"
echo
echo "ArgoCD:"
echo "  kubectl port-forward svc/argocd-server -n infraforge-argocd 8080:443"
echo "  URL: https://localhost:8080"
echo "  Username: admin"
echo "  Password: kubectl -n infraforge-argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
echo
echo "To check status:"
echo "  kubectl get pods -A | grep demo-"
echo "  kubectl get applications -n infraforge-argocd"
echo
echo -e "${YELLOW}Note: Services may take a few minutes to fully deploy.${NC}"
echo -e "${YELLOW}You can monitor progress with: watch 'kubectl get pods -n demo-${ENVIRONMENT}'${NC}"
echo