#!/bin/bash

# Fix Platform Services Deployment Script
# This script fixes the GitOps deployment issues for PostgreSQL and Redis

echo "=== Fixing Platform Services Deployment ==="

# Step 1: Build and push the updated operator
echo "Step 1: Building updated operator..."
cd /Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform/infrastructure/platform-operator

# Build the Docker image
docker build -t ghcr.io/nimbusprotch/platform-operator:latest -f Dockerfile .

# Push to registry (requires authentication)
docker push ghcr.io/nimbusprotch/platform-operator:latest

# Step 2: Update CRDs
echo "Step 2: Updating CRDs..."
make manifests
kubectl apply -f config/crd/bases/

# Step 3: Restart operator to pick up changes
echo "Step 3: Restarting platform operator..."
kubectl rollout restart deployment/platform-operator-controller-manager -n platform-operator-system

# Wait for operator to be ready
kubectl wait --for=condition=available --timeout=300s deployment/platform-operator-controller-manager -n platform-operator-system

# Step 4: Delete and recreate the claim to trigger reconciliation
echo "Step 4: Recreating platform claim..."
kubectl delete -f /Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform/deployments/dev/platform-infrastructure-claim.yaml --ignore-not-found
sleep 5
kubectl apply -f /Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform/deployments/dev/platform-infrastructure-claim.yaml

# Step 5: Wait for ApplicationSet to be created
echo "Step 5: Waiting for ApplicationSet..."
sleep 10
kubectl get applicationset -n argocd

# Step 6: Sync ArgoCD applications
echo "Step 6: Syncing ArgoCD applications..."
# Get all platform service applications
apps=$(kubectl get applications -n argocd -o name | grep -E "(product-db|user-db|redis)")

for app in $apps; do
    app_name=$(echo $app | cut -d'/' -f2)
    echo "Syncing $app_name..."
    kubectl patch application $app_name -n argocd --type merge -p '{"operation": {"sync": {"prune": true, "revision": "HEAD"}}}'
done

# Step 7: Check deployment status
echo "Step 7: Checking deployment status..."
sleep 30

echo ""
echo "=== Platform Services Status ==="
kubectl get pods -n dev-platform

echo ""
echo "=== PVC Status ==="
kubectl get pvc -n dev-platform

echo ""
echo "=== PostgreSQL Clusters ==="
kubectl get clusters.postgresql.cnpg.io -n dev-platform

echo ""
echo "=== Redis Instances ==="
kubectl get redisfailovers -n dev-platform

echo ""
echo "=== ArgoCD Applications ==="
kubectl get applications -n argocd | grep -E "(product-db|user-db|redis)"

echo ""
echo "=== Fix Complete ==="
echo "If services are still not deploying, check:"
echo "1. ArgoCD application logs: kubectl logs -n argocd deployment/argocd-application-controller"
echo "2. Operator logs: kubectl logs -n platform-operator-system deployment/platform-operator-controller-manager"
echo "3. ChartMuseum charts: curl http://localhost:8080/api/charts"