#!/bin/bash

# Platform Setup Script - Single Command Setup
# This script will setup the entire platform from scratch

set -e

echo "========================================="
echo "    InfraForge Platform Setup Script"
echo "========================================="
echo

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Step 1: Create Kind cluster
echo -e "${BLUE}Step 1: Creating Kind cluster with 4 nodes...${NC}"
cat > kind-config.yaml << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
- role: worker
- role: worker
- role: worker
EOF

kind delete cluster --name infraforge-cluster 2>/dev/null || true
kind create cluster --name infraforge-cluster --config kind-config.yaml
echo -e "${GREEN}✓ Kind cluster created${NC}"

# Step 2: Install Operators
echo -e "${BLUE}Step 2: Installing operators...${NC}"

# CloudNativePG
kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.23/releases/cnpg-1.23.0.yaml

# RabbitMQ Operator
kubectl apply -f https://github.com/rabbitmq/cluster-operator/releases/latest/download/cluster-operator.yml

# Redis Operator
helm repo add ot-helm https://ot-container-kit.github.io/helm-charts/ 2>/dev/null || true
helm repo update
helm install redis-operator ot-helm/redis-operator --create-namespace --namespace redis-operator-system

# MinIO Operator
kubectl apply -k 'https://github.com/minio/operator/resources?ref=v6.0.0'

echo -e "${GREEN}✓ Operators installed${NC}"

# Step 3: Install ArgoCD
echo -e "${BLUE}Step 3: Installing ArgoCD...${NC}"
kubectl create namespace infraforge-argocd
kubectl apply -n infraforge-argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for ArgoCD to be ready
kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n infraforge-argocd
echo -e "${GREEN}✓ ArgoCD installed${NC}"

# Step 4: Create namespaces
echo -e "${BLUE}Step 4: Creating namespaces...${NC}"
kubectl create namespace demo-dev
kubectl create namespace demo-prod
echo -e "${GREEN}✓ Namespaces created${NC}"

# Step 5: Configure ArgoCD
echo -e "${BLUE}Step 5: Configuring ArgoCD...${NC}"

# Create AppProjects
kubectl apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infraforge-dev
  namespace: infraforge-argocd
spec:
  description: Development environment for InfraForge platform
  sourceRepos:
  - 'https://github.com/NimbusProTch/PaaS-Platform.git'
  destinations:
  - namespace: 'demo-dev'
    server: 'https://kubernetes.default.svc'
  clusterResourceWhitelist:
  - group: '*'
    kind: '*'
  namespaceResourceWhitelist:
  - group: '*'
    kind: '*'
---
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infraforge-prod
  namespace: infraforge-argocd
spec:
  description: Production environment for InfraForge platform
  sourceRepos:
  - 'https://github.com/NimbusProTch/PaaS-Platform.git'
  destinations:
  - namespace: 'demo-prod'
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

# Step 6: Deploy ApplicationSets
echo -e "${BLUE}Step 6: Deploying ApplicationSets...${NC}"
kubectl apply -f manifests/platform-cluster/appsets/dev/operator.yaml
kubectl apply -f manifests/platform-cluster/appsets/prod/operator.yaml

# Restart ApplicationSet controller to pick up new apps
kubectl rollout restart deployment/argocd-applicationset-controller -n infraforge-argocd

echo -e "${GREEN}✓ ApplicationSets deployed${NC}"

# Step 7: Wait for applications to sync
echo -e "${BLUE}Step 7: Waiting for applications to sync...${NC}"
sleep 30

# Force refresh all apps
kubectl get applications -n infraforge-argocd -o name | while read app; do
  kubectl -n infraforge-argocd patch $app --type merge -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}'
done

# Enable auto-sync
kubectl get applications -n infraforge-argocd -o name | while read app; do
  kubectl -n infraforge-argocd patch $app --type merge -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'
done

echo -e "${GREEN}✓ Applications syncing${NC}"

# Step 8: Final status check
echo -e "${BLUE}Step 8: Checking deployment status...${NC}"
echo
echo "Waiting for all pods to be ready (this may take a few minutes)..."
sleep 60

echo
echo "========================================="
echo "        DEPLOYMENT STATUS"
echo "========================================="
echo

echo "DEV ENVIRONMENT (demo-dev):"
kubectl get pods -n demo-dev --no-headers 2>/dev/null | awk '{print "  "$1": "$3}' || echo "  Pods still initializing..."

echo
echo "PROD ENVIRONMENT (demo-prod):"
kubectl get pods -n demo-prod --no-headers 2>/dev/null | awk '{print "  "$1": "$3}' || echo "  Pods still initializing..."

echo
echo -e "${GREEN}========================================="
echo -e "    Platform setup complete!"
echo -e "=========================================${NC}"
echo
echo "Access ArgoCD:"
echo "  kubectl port-forward svc/argocd-server -n infraforge-argocd 8080:443"
echo "  URL: https://localhost:8080"
echo "  Username: admin"
echo "  Password: kubectl -n infraforge-argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
echo
echo "To check status:"
echo "  kubectl get pods -A | grep demo-"
echo "  kubectl get applications -n infraforge-argocd"
echo