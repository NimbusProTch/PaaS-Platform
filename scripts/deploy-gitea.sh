#!/bin/bash
set -e

echo "📦 Deploying Gitea..."

# Create namespace
kubectl create namespace gitea --dry-run=client -o yaml | kubectl apply -f -

# Deploy Gitea using Helm
helm repo add gitea-charts https://dl.gitea.io/charts/ 2>/dev/null || true
helm repo update

helm upgrade --install gitea gitea-charts/gitea \
  --namespace gitea \
  --set service.http.type=NodePort \
  --set service.http.nodePort=30300 \
  --set gitea.admin.username=gitea_admin \
  --set gitea.admin.password=r8sA8CPHD9!bt6d \
  --set gitea.admin.email=gitea@local.domain \
  --set persistence.enabled=false \
  --set postgresql-ha.enabled=false \
  --set postgresql.enabled=false \
  --set redis-cluster.enabled=false \
  --set gitea.config.database.DB_TYPE=sqlite3 \
  --set gitea.config.cache.ADAPTER=memory \
  --set gitea.config.session.PROVIDER=memory \
  --set gitea.config.server.ROOT_URL=http://gitea-http.gitea.svc.cluster.local:3000 \
  --set gitea.config.server.DOMAIN=gitea-http.gitea.svc.cluster.local \
  --set gitea.config.service.DISABLE_REGISTRATION=true \
  --set gitea.config.api.ENABLE_SWAGGER=false \
  --wait --timeout 5m

echo "✅ Gitea deployed"

# Initialize Gitea token
echo ""
echo "🔑 Initializing Gitea token..."
kubectl delete job gitea-token-init -n gitea 2>/dev/null || true
kubectl apply -f "$(dirname "$0")/init-gitea-token.yaml"

echo "⏳ Waiting for token initialization to complete..."
kubectl wait --for=condition=complete --timeout=120s job/gitea-token-init -n gitea

if kubectl get secret gitea-token -n platform-operator-system >/dev/null 2>&1; then
  echo "✅ Gitea token initialized successfully"
else
  echo "❌ Failed to initialize Gitea token"
  kubectl logs -n gitea -l job-name=gitea-token-init --tail=50
  exit 1
fi

echo ""
echo "✅ Gitea setup complete!"
echo ""
echo "Access Gitea at: http://localhost:30300 (NodePort)"
echo "Username: gitea_admin"
echo "Password: r8sA8CPHD9!bt6d"
echo ""
echo "🔑 Token stored in Secret: gitea-token (platform-operator-system namespace)"
