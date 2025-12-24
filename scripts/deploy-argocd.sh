#!/bin/bash
set -e

echo "📦 Deploying ArgoCD..."

# Create namespace
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -

# Install ArgoCD
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for ArgoCD to be ready
echo "⏳ Waiting for ArgoCD to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd

# Get admin password
ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)

echo "✅ ArgoCD deployed"
echo ""
echo "Port-forward to access ArgoCD:"
echo "  kubectl port-forward svc/argocd-server -n argocd 8081:443"
echo ""
echo "Username: admin"
echo "Password: ${ARGOCD_PASSWORD}"
