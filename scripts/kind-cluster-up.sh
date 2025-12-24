#!/bin/bash
set -e

CLUSTER_NAME=${1:-platform-test}

echo "🚀 Creating Kind cluster: ${CLUSTER_NAME}"

# Create cluster with kind
cat <<EOF | kind create cluster --name ${CLUSTER_NAME} --config=-
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
    hostPort: 8080
    protocol: TCP
  - containerPort: 443
    hostPort: 8443
    protocol: TCP
  - containerPort: 3000
    hostPort: 3000
    protocol: TCP
EOF

echo "✅ Kind cluster created: ${CLUSTER_NAME}"
kubectl cluster-info --context kind-${CLUSTER_NAME}
