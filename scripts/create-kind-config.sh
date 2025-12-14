#!/bin/bash
# Create Kind config for InfraForge

CLUSTER_NAME="${1:-infraforge-cluster}"

cat > /tmp/infraforge-kind-config.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${CLUSTER_NAME}
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "infraforge.io/platform=true,environment=platform"
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        enable-admission-plugins: NodeRestriction,ResourceQuota
  extraPortMappings:
  - containerPort: 30000
    hostPort: 30000
    protocol: TCP
  - containerPort: 30001
    hostPort: 30001
    protocol: TCP
EOF

echo "/tmp/infraforge-kind-config.yaml"