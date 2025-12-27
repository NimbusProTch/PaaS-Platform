# GitHub Actions Workflows

## Active Workflows

### 1. build-microservices.yml
- **Trigger:** Push to main/develop, changes in microservices/
- **Purpose:** Build multi-arch Docker images for microservices
- **Output:** Push to ghcr.io with tags: latest, sha, timestamp

### 2. build-operator.yml
- **Trigger:** Push to main/develop, changes in infrastructure/platform-operator/
- **Purpose:** Build multi-arch Docker image for platform operator
- **Output:** Push to ghcr.io with versioned tags

### 3. chart-publish.yml
- **Trigger:** Push to main/develop, changes in charts/
- **Purpose:** Package and publish Helm charts to OCI registry
- **Output:** Push charts to ghcr.io OCI registry

## Workflow Strategy

All workflows:
- Support multi-architecture builds (linux/amd64, linux/arm64)
- Use GitHub Container Registry (ghcr.io)
- Auto-trigger on relevant path changes
- Use semantic versioning where applicable