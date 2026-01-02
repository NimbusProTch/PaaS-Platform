#!/bin/bash
set -e

# Script to push all Helm charts to Gitea charts repository
# This eliminates the need for ChartMuseum

GITEA_URL="${GITEA_URL:-http://gitea-http.gitea.svc.cluster.local:3000}"
GITEA_ORG="${GITEA_ORG:-infraforge}"
GITEA_CHARTS_REPO="${GITEA_CHARTS_REPO:-charts}"
GITEA_TOKEN="${GITEA_TOKEN}"
GITEA_USERNAME="${GITEA_USERNAME:-gitea_admin}"

if [ -z "$GITEA_TOKEN" ]; then
  echo "Error: GITEA_TOKEN environment variable is required"
  exit 1
fi

CHARTS_DIR="/Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform/charts"
TEMP_DIR=$(mktemp -d)

echo "==================================="
echo "Pushing Helm Charts to Gitea"
echo "==================================="
echo "Gitea URL: $GITEA_URL"
echo "Organization: $GITEA_ORG"
echo "Repository: $GITEA_CHARTS_REPO"
echo "Charts Directory: $CHARTS_DIR"
echo "Temp Directory: $TEMP_DIR"
echo "==================================="

# Create organization if it doesn't exist
echo "[1/5] Creating organization '$GITEA_ORG'..."
curl -X POST -s -o /dev/null -w "%{http_code}\n" \
  "$GITEA_URL/api/v1/orgs" \
  -H "Authorization: token $GITEA_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"username\": \"$GITEA_ORG\",
    \"description\": \"Infrastructure Platform Organization\",
    \"visibility\": \"public\"
  }" | grep -E "201|422" > /dev/null && echo "Organization ready" || echo "Organization creation failed (may already exist)"

# Create charts repository if it doesn't exist
echo "[2/5] Creating repository '$GITEA_CHARTS_REPO'..."
curl -X POST -s -o /dev/null -w "%{http_code}\n" \
  "$GITEA_URL/api/v1/orgs/$GITEA_ORG/repos" \
  -H "Authorization: token $GITEA_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"$GITEA_CHARTS_REPO\",
    \"description\": \"Helm Charts Repository\",
    \"private\": false,
    \"auto_init\": true,
    \"default_branch\": \"main\"
  }" | grep -E "201|409|422" > /dev/null && echo "Repository ready" || echo "Repository creation failed (may already exist)"

# Wait for repository to be ready
echo "[3/5] Waiting for repository initialization..."
sleep 5

# Clone the charts repository
CLONE_URL="http://$GITEA_USERNAME:$GITEA_TOKEN@${GITEA_URL#http://}/$GITEA_ORG/$GITEA_CHARTS_REPO.git"
echo "[4/5] Cloning charts repository..."
cd "$TEMP_DIR"
git clone "$CLONE_URL" repo || {
  echo "Failed to clone repository. Trying to initialize..."
  mkdir -p repo
  cd repo
  git init
  git remote add origin "$CLONE_URL"
}
cd repo

# Configure git
git config user.name "Platform Operator"
git config user.email "operator@platform.local"

# Try to checkout main branch or create it
git checkout main 2>/dev/null || git checkout -b main

# Copy all charts
echo "[5/5] Copying charts..."
for chart in "$CHARTS_DIR"/*/ ; do
  chart_name=$(basename "$chart")

  # Skip if not a directory or if it's a hidden directory
  if [ ! -d "$chart" ] || [ "${chart_name:0:1}" = "." ]; then
    continue
  fi

  # Skip packaged charts (.tgz files)
  if [ "${chart_name##*.}" = "tgz" ]; then
    continue
  fi

  echo "  - Copying chart: $chart_name"
  mkdir -p "$chart_name"
  cp -r "$chart"* "$chart_name/" 2>/dev/null || true
  git add "$chart_name"
done

# Add README
cat > README.md << 'EOF'
# Helm Charts Repository

This repository contains all Helm charts used by the Platform Operator.

## Available Charts

- **microservice** - Generic microservice deployment chart
- **postgresql** - CloudNative-PG PostgreSQL cluster
- **redis** - Redis cluster
- **mongodb** - MongoDB replica set
- **rabbitmq** - RabbitMQ cluster
- **kafka** - Kafka cluster

## Usage

Charts are referenced by ArgoCD Applications using Git source:

```yaml
source:
  repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/charts
  path: microservice
  targetRevision: main
  helm:
    valueFiles:
      - values.yaml
```

## Structure

Each chart directory contains:
- `Chart.yaml` - Chart metadata
- `values.yaml` - Default values
- `templates/` - Kubernetes manifests

## Managed By

This repository is managed by the Platform Operator.
All charts are automatically synchronized from the main repository.
EOF

git add README.md

# Commit and push
if git diff --staged --quiet; then
  echo "No changes to commit"
else
  echo "Committing changes..."
  git commit -m "Add Helm charts from Platform Operator"

  echo "Pushing to Gitea..."
  git push -u origin main || git push origin main
fi

# Cleanup
cd /
rm -rf "$TEMP_DIR"

echo "==================================="
echo "Charts successfully pushed to Gitea!"
echo "Repository URL: $GITEA_URL/$GITEA_ORG/$GITEA_CHARTS_REPO"
echo "==================================="
