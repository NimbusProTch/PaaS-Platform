#!/bin/bash
# Fix ArgoCD installation using Helm

set -euo pipefail

echo "🔧 Fixing ArgoCD installation..."

# Delete current ArgoCD
echo "Removing current ArgoCD installation..."
kubectl delete namespace infraforge-argocd --wait=false 2>/dev/null || true

# Wait for namespace deletion
echo "Waiting for namespace to be deleted..."
while kubectl get namespace infraforge-argocd 2>/dev/null; do
    echo -n "."
    sleep 2
done
echo ""

# Install ArgoCD using Helm
echo "Installing ArgoCD with Helm..."
helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
helm repo update

# Create namespace
kubectl create namespace infraforge-argocd

# Install ArgoCD
helm install argocd argo/argo-cd \
  --namespace infraforge-argocd \
  --version 7.1.3 \
  --set configs.params."server\.insecure"=true \
  --set configs.cm.url="https://argocd-server.infraforge-argocd.svc.cluster.local:443" \
  --set configs.cm."timeout\.reconciliation"="180s" \
  --wait

echo "✅ ArgoCD fixed successfully!"

# Get admin password
echo ""
echo "ArgoCD admin password:"
kubectl -n infraforge-argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
echo ""