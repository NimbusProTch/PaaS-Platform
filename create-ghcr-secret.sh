#!/bin/bash

# GitHub token'ınızı buraya girin
GITHUB_TOKEN="${GITHUB_TOKEN:-ghp_YOUR_GITHUB_TOKEN_HERE}"

if [ "$GITHUB_TOKEN" == "ghp_YOUR_GITHUB_TOKEN_HERE" ]; then
    echo "Lütfen GITHUB_TOKEN environment variable'ı ayarlayın:"
    echo "export GITHUB_TOKEN=ghp_YOUR_ACTUAL_TOKEN"
    exit 1
fi

# Base64 encode for Docker auth
DOCKER_AUTH_BASE64=$(echo -n "nimbusprotch:$GITHUB_TOKEN" | base64)

# Create namespace if not exists
kubectl create namespace platform-operator-system --dry-run=client -o yaml | kubectl apply -f -

# Create GitHub token secret
kubectl create secret generic github-token \
  --from-literal=token="$GITHUB_TOKEN" \
  -n platform-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Create Docker registry secret for GHCR
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=nimbusprotch \
  --docker-password="$GITHUB_TOKEN" \
  --docker-email=admin@infraforge.io \
  -n platform-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secrets created successfully!"

# Verify secrets
echo -e "\nVerifying secrets:"
kubectl get secrets -n platform-operator-system | grep -E "(github-token|ghcr-secret)"