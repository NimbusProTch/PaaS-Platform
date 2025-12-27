# Scripts Directory

This directory contains utility scripts for managing the PaaS Platform infrastructure and deployments.

## Scripts Overview

### Kubernetes Cluster Management
- **`kind-cluster-up.sh`** - Creates a local Kind Kubernetes cluster with custom configuration
- **`kind-cluster-down.sh`** - Destroys the local Kind cluster

### Component Deployment
- **`deploy-gitea.sh`** - Deploys Gitea Git server using Helm
- **`deploy-argocd.sh`** - Deploys ArgoCD GitOps controller
- **`deploy-operator.sh`** - Deploys the Platform Operator with CRDs

### Secret Management
- **`create-ghcr-secret.sh`** - Creates GitHub Container Registry pull secrets
- **`create-all-secrets.sh`** - Creates all required secrets for platform operation

### Configuration Files
- **`init-gitea-token.yaml`** - Kubernetes Job for initializing Gitea access token

## Usage

All scripts should be executed from the repository root directory:

```bash
./scripts/kind-cluster-up.sh
./scripts/deploy-gitea.sh
# etc...
```

**Note:** For most operations, use the Makefile in the repository root instead of running scripts directly:

```bash
make kind-create    # Instead of ./scripts/kind-cluster-up.sh
make install-gitea  # Instead of ./scripts/deploy-gitea.sh
make full-deploy    # Full automated deployment
```

The Makefile provides better error handling, dependency management, and status reporting.