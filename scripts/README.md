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

### GitOps Utilities
- **`push-charts-to-gitea.sh`** - Pushes all Helm charts to Gitea charts repository (Git-only architecture)
- **`setup-gitea.sh`** - Initializes Gitea with organizations and repositories
- **`make-charts-public.sh`** - Makes chart repositories public

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

---

## Script Details

### push-charts-to-gitea.sh

**Purpose**: Migrate Helm charts to Git-only architecture by pushing them to Gitea.

**What it does**:
1. Creates `infraforge` organization in Gitea (if not exists)
2. Creates `charts` repository (if not exists)
3. Copies all charts from `/charts` directory to Gitea
4. Commits and pushes to main branch

**Environment Variables**:
- `GITEA_TOKEN` - Gitea access token (required)
- `GITEA_URL` - Gitea server URL (default: http://gitea-http.gitea.svc.cluster.local:3000)
- `GITEA_ORG` - Organization name (default: infraforge)
- `GITEA_CHARTS_REPO` - Repository name (default: charts)
- `GITEA_USERNAME` - Username (default: gitea_admin)

**Usage**:
```bash
# Get token from Kubernetes
export GITEA_TOKEN=$(kubectl get secret -n gitea gitea-admin-secret -o jsonpath='{.data.password}' | base64 -d)

# Run script
./scripts/push-charts-to-gitea.sh
```

**See also**: `/docs/ARCHITECTURE-GIT-ONLY.md` for complete architecture documentation.