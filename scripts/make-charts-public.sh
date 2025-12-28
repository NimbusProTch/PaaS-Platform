#!/bin/bash

# Make GitHub Packages charts public
# Requires gh CLI and authentication

echo "🔓 Making Helm charts public in GitHub Packages..."

# Charts to make public
CHARTS=(
  "microservice"
  "postgresql"
  "redis"
  "rabbitmq"
  "mongodb"
  "kafka"
)

ORG="nimbusprotch"

for CHART in "${CHARTS[@]}"; do
  echo "Making $CHART public..."

  # GitHub API call to change visibility
  curl -X PATCH \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    https://api.github.com/user/packages/container/${CHART}/versions \
    -d '{"visibility":"public"}'

  echo "✅ $CHART is now public"
done

echo "🎉 All charts are now public!"
echo "ArgoCD can now pull without authentication"