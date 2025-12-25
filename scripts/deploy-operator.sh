#!/bin/bash
set -e

CLUSTER_NAME=${1:-platform-test}
IMG=${2:-platform-operator:latest}

echo "🔨 Building platform operator..."
cd "$(dirname "$0")/.."

# Build the operator image with correct context
docker build \
  -f infrastructure/platform-operator/Dockerfile \
  -t ${IMG} \
  infrastructure/platform-operator

echo "📦 Loading image into Kind cluster: ${CLUSTER_NAME}..."
kind load docker-image ${IMG} --name ${CLUSTER_NAME}

echo "📋 Installing CRDs..."
kubectl apply -f infrastructure/platform-operator/config/crd/bases

echo "🚀 Deploying operator..."

# Create namespace
kubectl create namespace platform-operator-system --dry-run=client -o yaml | kubectl apply -f -

# Apply RBAC
kubectl apply -f infrastructure/platform-operator/config/default/rbac.yaml -n platform-operator-system

# Apply operator deployment
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "${PROJECT_ROOT}/infrastructure/platform-operator/config/manager"
kustomize edit set image controller=${IMG}

kubectl apply -k . -n platform-operator-system

cd "${PROJECT_ROOT}"

echo "⏳ Waiting for operator to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/platform-operator-controller-manager -n platform-operator-system || true

echo "✅ Platform operator deployed"
echo ""
echo "Check operator logs:"
echo "  kubectl logs -n platform-operator-system -l control-plane=controller-manager -f"
