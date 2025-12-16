#!/bin/bash

# Install EKS Components Script
# This script installs all necessary components on EKS cluster

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration from environment or defaults
CLUSTER_NAME=${CLUSTER_NAME:-infraforge-dev}
AWS_REGION=${AWS_REGION:-eu-west-1}
ENVIRONMENT=${ENVIRONMENT:-dev}

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}   Installing EKS Platform Components${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Cluster: ${GREEN}${CLUSTER_NAME}${NC}"
echo -e "Region: ${GREEN}${AWS_REGION}${NC}"
echo -e "Environment: ${GREEN}${ENVIRONMENT}${NC}"
echo

# Function to check if a helm release exists
helm_release_exists() {
    helm list -n $2 2>/dev/null | grep -q $1
}

# Function to check if a namespace exists
namespace_exists() {
    kubectl get namespace $1 &> /dev/null
}

# Step 1: Install Metrics Server
echo -e "${BLUE}Installing Metrics Server...${NC}"
if ! helm_release_exists "metrics-server" "kube-system"; then
    helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
    helm repo update
    helm install metrics-server metrics-server/metrics-server \
        --namespace kube-system \
        --set args='{--cert-dir=/tmp,--kubelet-preferred-address-types=InternalIP\,ExternalIP\,Hostname,--kubelet-use-node-status-port,--metric-resolution=15s,--kubelet-insecure-tls}'
    echo -e "${GREEN}✓ Metrics Server installed${NC}"
else
    echo -e "${YELLOW}⚠ Metrics Server already installed${NC}"
fi

# Step 2: Install Cluster Autoscaler
echo -e "${BLUE}Installing Cluster Autoscaler...${NC}"
if ! helm_release_exists "cluster-autoscaler" "kube-system"; then
    helm repo add autoscaler https://kubernetes.github.io/autoscaler
    helm repo update

    # Get the IAM role ARN (you might need to adjust this based on your setup)
    AUTOSCALER_ROLE_ARN=$(aws iam list-roles --query "Roles[?contains(RoleName, '${CLUSTER_NAME}-cluster-autoscaler')].Arn" --output text)

    helm install cluster-autoscaler autoscaler/cluster-autoscaler \
        --namespace kube-system \
        --set autoDiscovery.clusterName=${CLUSTER_NAME} \
        --set awsRegion=${AWS_REGION} \
        --set rbac.serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="${AUTOSCALER_ROLE_ARN}" \
        --set extraArgs.balance-similar-node-groups=true \
        --set extraArgs.skip-nodes-with-local-storage=false
    echo -e "${GREEN}✓ Cluster Autoscaler installed${NC}"
else
    echo -e "${YELLOW}⚠ Cluster Autoscaler already installed${NC}"
fi

# Step 3: Install AWS Load Balancer Controller
echo -e "${BLUE}Installing AWS Load Balancer Controller...${NC}"
if ! helm_release_exists "aws-load-balancer-controller" "kube-system"; then
    helm repo add eks https://aws.github.io/eks-charts
    helm repo update

    # Get VPC ID
    VPC_ID=$(aws eks describe-cluster --name ${CLUSTER_NAME} --region ${AWS_REGION} --query "cluster.resourcesVpcConfig.vpcId" --output text)

    # Get the IAM role ARN
    LB_ROLE_ARN=$(aws iam list-roles --query "Roles[?contains(RoleName, '${CLUSTER_NAME}-aws-load-balancer-controller')].Arn" --output text)

    helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
        --namespace kube-system \
        --set clusterName=${CLUSTER_NAME} \
        --set region=${AWS_REGION} \
        --set vpcId=${VPC_ID} \
        --set serviceAccount.create=true \
        --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="${LB_ROLE_ARN}"
    echo -e "${GREEN}✓ AWS Load Balancer Controller installed${NC}"
else
    echo -e "${YELLOW}⚠ AWS Load Balancer Controller already installed${NC}"
fi

# Step 4: Install Kong API Gateway with Gateway API support
echo -e "${BLUE}Installing Kong API Gateway...${NC}"
if ! namespace_exists "kong"; then
    kubectl create namespace kong
fi

if ! helm_release_exists "kong" "kong"; then
    helm repo add kong https://charts.konghq.com
    helm repo update

    cat <<EOF > /tmp/kong-values.yaml
image:
  repository: kong/kong-gateway
  tag: "3.5"

env:
  database: "off"
  nginx_worker_processes: "2"
  proxy_access_log: /dev/stdout
  admin_access_log: /dev/stdout
  proxy_error_log: /dev/stderr
  admin_error_log: /dev/stderr

proxy:
  enabled: true
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"

admin:
  enabled: true
  type: ClusterIP

ingressController:
  enabled: true
  gatewayAPI:
    enabled: true
  env:
    publish_service: kong/kong-proxy

gateway:
  enabled: true

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80

resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi
EOF

    helm install kong kong/kong --namespace kong --values /tmp/kong-values.yaml
    echo -e "${GREEN}✓ Kong API Gateway installed${NC}"
else
    echo -e "${YELLOW}⚠ Kong API Gateway already installed${NC}"
fi

# Step 5: Install Gateway API CRDs
echo -e "${BLUE}Installing Gateway API CRDs...${NC}"
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml
echo -e "${GREEN}✓ Gateway API CRDs installed${NC}"

# Step 6: Apply Kong Gateway configurations
echo -e "${BLUE}Applying Kong Gateway configurations...${NC}"
kubectl apply -f manifests/gateway-api/kong-gateway-class.yaml || true
echo -e "${GREEN}✓ Kong Gateway configurations applied${NC}"

# Step 7: Install Prometheus Stack (optional for dev)
if [ "$ENVIRONMENT" = "prod" ] || [ "$INSTALL_MONITORING" = "true" ]; then
    echo -e "${BLUE}Installing Prometheus Stack...${NC}"
    if ! namespace_exists "monitoring"; then
        kubectl create namespace monitoring
    fi

    if ! helm_release_exists "kube-prometheus-stack" "monitoring"; then
        helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
        helm repo update

        cat <<EOF > /tmp/prometheus-values.yaml
prometheus:
  prometheusSpec:
    retention: 7d
    storageSpec:
      volumeClaimTemplate:
        spec:
          storageClassName: gp3
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 20Gi

grafana:
  enabled: true
  adminPassword: "admin-${RANDOM}"
  persistence:
    enabled: true
    storageClassName: gp3
    size: 10Gi
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"

alertmanager:
  alertmanagerSpec:
    storage:
      volumeClaimTemplate:
        spec:
          storageClassName: gp3
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 10Gi
EOF

        helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
            --namespace monitoring \
            --values /tmp/prometheus-values.yaml
        echo -e "${GREEN}✓ Prometheus Stack installed${NC}"

        # Get Grafana password
        echo -e "${YELLOW}Grafana admin password saved to: /tmp/grafana-password.txt${NC}"
        kubectl get secret -n monitoring kube-prometheus-stack-grafana -o jsonpath="{.data.admin-password}" | base64 -d > /tmp/grafana-password.txt
    else
        echo -e "${YELLOW}⚠ Prometheus Stack already installed${NC}"
    fi
fi

# Step 8: Install External DNS (optional)
if [ "$ENABLE_EXTERNAL_DNS" = "true" ]; then
    echo -e "${BLUE}Installing External DNS...${NC}"
    if ! namespace_exists "external-dns"; then
        kubectl create namespace external-dns
    fi

    if ! helm_release_exists "external-dns" "external-dns"; then
        helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
        helm repo update

        EXTERNAL_DNS_ROLE_ARN=$(aws iam list-roles --query "Roles[?contains(RoleName, '${CLUSTER_NAME}-external-dns')].Arn" --output text)

        helm install external-dns external-dns/external-dns \
            --namespace external-dns \
            --set provider=aws \
            --set aws.region=${AWS_REGION} \
            --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="${EXTERNAL_DNS_ROLE_ARN}" \
            --set policy=sync
        echo -e "${GREEN}✓ External DNS installed${NC}"
    else
        echo -e "${YELLOW}⚠ External DNS already installed${NC}"
    fi
fi

# Step 9: Install Cert Manager (optional)
if [ "$ENABLE_CERT_MANAGER" = "true" ]; then
    echo -e "${BLUE}Installing Cert Manager...${NC}"
    if ! namespace_exists "cert-manager"; then
        kubectl create namespace cert-manager
    fi

    if ! helm_release_exists "cert-manager" "cert-manager"; then
        helm repo add jetstack https://charts.jetstack.io
        helm repo update

        helm install cert-manager jetstack/cert-manager \
            --namespace cert-manager \
            --set installCRDs=true \
            --set global.leaderElection.namespace=cert-manager
        echo -e "${GREEN}✓ Cert Manager installed${NC}"

        # Create Let's Encrypt ClusterIssuer
        kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: platform@infraforge.io
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: kong
EOF
        echo -e "${GREEN}✓ Let's Encrypt ClusterIssuer created${NC}"
    else
        echo -e "${YELLOW}⚠ Cert Manager already installed${NC}"
    fi
fi

# Step 10: Create default storage classes
echo -e "${BLUE}Creating storage classes...${NC}"
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Delete
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3-retain
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Retain
EOF
echo -e "${GREEN}✓ Storage classes created${NC}"

# Step 11: Verify installations
echo -e "${BLUE}Verifying installations...${NC}"
echo

echo "Metrics Server:"
kubectl top nodes 2>/dev/null || echo "  Metrics not ready yet"
echo

echo "Kong API Gateway:"
kubectl get svc -n kong kong-proxy
echo

echo "Storage Classes:"
kubectl get storageclass
echo

if [ "$ENVIRONMENT" = "prod" ] || [ "$INSTALL_MONITORING" = "true" ]; then
    echo "Prometheus Stack:"
    kubectl get pods -n monitoring | grep -E "prometheus|grafana|alertmanager"
    echo
    echo "Grafana URL:"
    kubectl get svc -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
    echo
fi

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   EKS Components Installation Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo
echo "Next steps:"
echo "1. Wait for all pods to be ready"
echo "2. Configure DNS records for your domain"
echo "3. Deploy your applications"
echo
echo "Useful commands:"
echo "  kubectl get pods -A                    # Check all pods"
echo "  kubectl top nodes                      # Check node metrics"
echo "  kubectl get gateway -A                 # Check Gateway API resources"
echo "  kubectl get httproute -A               # Check HTTP routes"
echo "  kubectl get svc -n kong kong-proxy     # Get Kong LoadBalancer URL"
echo