#!/bin/bash
set -e

CLUSTER_NAME=${1:-platform-test}

echo "🗑️  Deleting Kind cluster: ${CLUSTER_NAME}"
kind delete cluster --name ${CLUSTER_NAME}
echo "✅ Cluster deleted"
