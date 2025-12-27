#!/bin/bash

# Check if GITHUB_TOKEN is set
if [ -z "$GITHUB_TOKEN" ]; then
    echo "ERROR: GITHUB_TOKEN environment variable is not set!"
    echo ""
    echo "Please set it first:"
    echo "export GITHUB_TOKEN=ghp_YOUR_TOKEN_HERE"
    exit 1
fi

echo "Using GitHub token: ${GITHUB_TOKEN:0:10}..."

# Create namespace if not exists
kubectl create namespace platform-operator-system --dry-run=client -o yaml | kubectl apply -f -

# 1. Create GitHub token secret (for Helm OCI registry access)
echo "Creating github-token secret..."
kubectl create secret generic github-token \
  --from-literal=token="$GITHUB_TOKEN" \
  -n platform-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -

# 2. Create Docker registry secret for GHCR (for pulling operator image)
echo "Creating ghcr-secret (docker registry secret)..."
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=nimbusprotch \
  --docker-password="$GITHUB_TOKEN" \
  --docker-email=admin@infraforge.io \
  -n platform-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Create Gitea token secret (for Git operations)
echo "Creating gitea-token secret..."
# Check if Gitea token already exists from previous setup
GITEA_TOKEN=$(kubectl get secret gitea-token -n gitea -o jsonpath='{.data.token}' 2>/dev/null | base64 -d)

if [ -z "$GITEA_TOKEN" ]; then
    echo "Warning: Gitea token not found in gitea namespace."
    echo "Using a placeholder token. You need to update this with actual Gitea admin token."
    GITEA_TOKEN="placeholder-update-with-actual-token"
fi

kubectl create secret generic gitea-token \
  --from-literal=token="$GITEA_TOKEN" \
  -n platform-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "✅ All secrets created successfully!"
echo ""
echo "Verifying secrets:"
kubectl get secrets -n platform-operator-system | grep -E "(github-token|ghcr-secret|gitea-token)"

echo ""
echo "Now you can apply the operator manifest:"
echo "kubectl apply -f /tmp/platform-operator.yaml"