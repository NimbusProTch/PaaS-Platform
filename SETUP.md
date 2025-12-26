# Platform Operator - Complete Setup Guide

## 🎯 Overview

This platform provides a fully automated GitOps-driven Platform-as-a-Service (PaaS) on Kubernetes. With a single CRD claim, you can deploy complex microservices architectures including databases, caches, and message queues.

### Key Features
- **Claim-Driven**: Single YAML file defines entire application stack
- **GitOps Native**: ArgoCD ApplicationSets for continuous deployment
- **OCI Registry**: Helm charts stored in GitHub Container Registry
- **Zero Hardcoding**: All configuration from CRDs
- **Multi-Environment**: Support for dev, staging, prod environments
- **Automated Setup**: Bootstrap controller generates all required manifests

## 📋 Prerequisites

1. **Kubernetes Cluster** (v1.25+)
   - Local: Kind, Minikube, or Docker Desktop
   - Cloud: EKS, GKE, AKS

2. **GitHub Token** with permissions:
   - `read:packages` - Pull Helm charts from GHCR
   - `write:packages` - Push images to GHCR (for CI/CD)

3. **kubectl** and **helm** CLI tools installed

## 🚀 Quick Start

### Step 1: Install ArgoCD

```bash
# Create namespace
kubectl create namespace argocd

# Install ArgoCD
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s

# Get admin password (optional - for UI access)
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
```

### Step 2: Install Gitea (Local Git Server)

```bash
# Add Gitea Helm repo
helm repo add gitea-charts https://dl.gitea.io/charts/
helm repo update

# Install Gitea
helm install gitea gitea-charts/gitea \
  --namespace gitea \
  --create-namespace \
  --set gitea.admin.username=gitea_admin \
  --set gitea.admin.password=gitea_admin \
  --set service.http.type=ClusterIP \
  --set persistence.enabled=false \
  --set postgresql.enabled=false \
  --set gitea.config.database.DB_TYPE=sqlite3

# Wait for Gitea to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=gitea -n gitea --timeout=300s
```

### Step 3: Install Platform Operator

```bash
# Create namespace
kubectl create namespace platform-operator-system

# Create GitHub token secret (replace with your token)
export GITHUB_TOKEN="ghp_YOUR_ACTUAL_TOKEN_HERE"

kubectl create secret generic github-token \
  --from-literal=token=$GITHUB_TOKEN \
  --namespace platform-operator-system

# Create image pull secret
kubectl create secret docker-registry ghcr-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=infraforge \
  --docker-password=$GITHUB_TOKEN \
  --namespace platform-operator-system

# Apply operator manifests
kubectl apply -f https://raw.githubusercontent.com/NimbusProTch/PaaS-Platform/main/infrastructure/platform-operator/config/crd/bases/platform.infraforge.io_bootstrapclaims.yaml
kubectl apply -f https://raw.githubusercontent.com/NimbusProTch/PaaS-Platform/main/infrastructure/platform-operator/config/crd/bases/platform.infraforge.io_applicationclaims.yaml
kubectl apply -f https://raw.githubusercontent.com/NimbusProTch/PaaS-Platform/main/infrastructure/platform-operator/config/crd/bases/platform.infraforge.io_platformapplicationclaims.yaml

# Deploy operator
kubectl apply -k https://github.com/NimbusProTch/PaaS-Platform/infrastructure/platform-operator/config/default
```

### Step 4: Bootstrap GitOps Structure

```bash
# Create bootstrap claim
cat <<EOF | kubectl apply -f -
apiVersion: platform.infraforge.io/v1
kind: BootstrapClaim
metadata:
  name: platform-bootstrap
  namespace: default
spec:
  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: infraforge
  chartsRepository:
    type: oci
    url: oci://ghcr.io/infraforge
    version: "1.0.0"
  repositories:
    voltran: voltran
  gitOps:
    branch: main
    clusterType: nonprod
    environments:
      - dev
      - staging
      - prod
EOF

# Wait for bootstrap to complete
kubectl wait --for=condition=Ready bootstrapclaim/platform-bootstrap --timeout=300s

# Check status
kubectl get bootstrapclaim platform-bootstrap -o yaml
```

### Step 5: Setup ArgoCD Integration

After bootstrap completes, the operator generates ArgoCD setup manifests in the Gitea repository. You need to apply these:

```bash
# Port-forward to Gitea
kubectl port-forward -n gitea svc/gitea-http 3000:3000 &

# Clone the voltran repo (use gitea_admin/gitea_admin for auth)
git clone http://localhost:3000/infraforge/voltran.git
cd voltran

# Update GitHub token in the manifests
export GITHUB_TOKEN="ghp_YOUR_ACTUAL_TOKEN_HERE"
export AUTH_BASE64=$(echo -n "infraforge:$GITHUB_TOKEN" | base64)

# Replace placeholders
sed -i "s/GITHUB_TOKEN/$GITHUB_TOKEN/g" argocd-setup/02-helm-oci-secret.yaml
sed -i "s/GITHUB_TOKEN/$GITHUB_TOKEN/g" argocd-setup/05-github-token-secret.yaml
sed -i "s/BASE64_ENCODED_USERNAME:TOKEN/$AUTH_BASE64/g" argocd-setup/05-github-token-secret.yaml

# Apply ArgoCD setup manifests
kubectl apply -f argocd-setup/

# Verify setup
kubectl get secrets -n argocd | grep -E "gitea-repo|helm-oci-creds"
kubectl get applications -n argocd
```

### Step 6: Deploy Applications

Now you can deploy applications using claims:

```bash
# Deploy minimal microservices
cat <<EOF | kubectl apply -f -
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: demo-apps
  namespace: default
spec:
  environment: dev
  clusterType: nonprod
  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: infraforge
  applications:
    - name: product-service
      chart:
        name: microservice
        version: "1.0.0"
      image:
        repository: ghcr.io/infraforge/product-service
        tag: latest
      replicas: 2
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
    - name: user-service
      chart:
        name: microservice
        version: "1.0.0"
      image:
        repository: ghcr.io/infraforge/user-service
        tag: latest
      replicas: 2
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
EOF

# Deploy platform services
cat <<EOF | kubectl apply -f -
apiVersion: platform.infraforge.io/v1
kind: PlatformApplicationClaim
metadata:
  name: demo-platform
  namespace: default
spec:
  environment: dev
  clusterType: nonprod
  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: infraforge
  services:
    - type: postgresql
      name: main-db
      chart:
        name: postgresql
        version: "1.0.0"
      values:
        storage:
          size: 10Gi
    - type: redis
      name: cache
      chart:
        name: redis
        version: "1.0.0"
EOF

# Check status
kubectl get applicationclaims
kubectl get platformapplicationclaims
```

## 🔍 Verification

### Check Operator Logs
```bash
kubectl logs -n platform-operator-system -l control-plane=controller-manager -f
```

### Check ArgoCD Applications
```bash
kubectl get applications -n argocd
kubectl get applicationsets -n argocd
```

### Access ArgoCD UI
```bash
# Port-forward
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Open browser: https://localhost:8080
# Username: admin
# Password: (from Step 1)
```

### Access Gitea UI
```bash
# Port-forward
kubectl port-forward svc/gitea-http -n gitea 3000:3000

# Open browser: http://localhost:3000
# Username: gitea_admin
# Password: gitea_admin
```

## 🏗️ Architecture Components

### 1. **Bootstrap Controller**
- Creates Gitea organization and repositories
- Generates GitOps directory structure
- Creates ArgoCD setup manifests

### 2. **ApplicationClaim Controller**
- Manages microservice deployments
- Generates ApplicationSets for apps
- Handles Helm values merging

### 3. **PlatformApplicationClaim Controller**
- Manages infrastructure services (databases, caches)
- Creates operator-backed resources
- Handles platform-specific configurations

### 4. **GitOps Flow**
```
CRD Claim → Controller → Gitea (values) + OCI (charts) → ArgoCD → Kubernetes
```

## 📝 Configuration Reference

### BootstrapClaim Fields
| Field | Description | Example |
|-------|-------------|---------|
| `giteaURL` | Gitea server URL | `http://gitea-http.gitea.svc.cluster.local:3000` |
| `organization` | Git organization name | `infraforge` |
| `chartsRepository.type` | Repository type | `oci` |
| `chartsRepository.url` | OCI registry URL | `oci://ghcr.io/infraforge` |
| `repositories.voltran` | GitOps repo name | `voltran` |
| `gitOps.branch` | Git branch | `main` |
| `gitOps.clusterType` | Cluster type | `nonprod` |
| `gitOps.environments` | Environment list | `[dev, staging, prod]` |

### ApplicationClaim Fields
| Field | Description | Example |
|-------|-------------|---------|
| `environment` | Target environment | `dev` |
| `clusterType` | Cluster type | `nonprod` |
| `applications[].name` | App name | `product-service` |
| `applications[].chart.name` | Helm chart | `microservice` |
| `applications[].image.repository` | Docker image | `ghcr.io/infraforge/product-service` |
| `applications[].replicas` | Pod count | `2` |

## 🐛 Troubleshooting

### Issue: ImagePullBackOff
```bash
# Check if pull secret exists
kubectl get secret ghcr-pull-secret -n platform-operator-system

# Recreate if needed
kubectl delete secret ghcr-pull-secret -n platform-operator-system
kubectl create secret docker-registry ghcr-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=infraforge \
  --docker-password=$GITHUB_TOKEN \
  --namespace platform-operator-system

# Restart operator
kubectl rollout restart deployment/platform-operator-controller-manager -n platform-operator-system
```

### Issue: ArgoCD Can't Pull Charts
```bash
# Check OCI credentials
kubectl get secret helm-oci-creds -n argocd -o yaml

# Update if needed
kubectl delete secret helm-oci-creds -n argocd
kubectl apply -f voltran/argocd-setup/02-helm-oci-secret.yaml
```

### Issue: Bootstrap Fails
```bash
# Check operator logs
kubectl logs -n platform-operator-system -l control-plane=controller-manager --tail=100

# Check Gitea connectivity
kubectl exec -it deploy/platform-operator-controller-manager -n platform-operator-system -- curl http://gitea-http.gitea.svc.cluster.local:3000
```

## 🚧 Known Limitations

1. **Manual ArgoCD Setup**: After bootstrap, ArgoCD manifests must be applied manually
2. **Token Management**: GitHub tokens must be manually configured in secrets
3. **Single Cluster**: Current setup assumes single cluster deployment
4. **No RBAC**: AppProjects and RBAC policies not automated yet
5. **No Monitoring**: Prometheus/Grafana stack not included

## 🔄 Next Steps

1. **Production Deployment**
   - Use external database for Gitea
   - Enable persistence for all services
   - Configure TLS/ingress
   - Set up monitoring stack

2. **Multi-Tenancy**
   - Create AppProjects per team
   - Configure RBAC policies
   - Implement resource quotas

3. **CI/CD Integration**
   - Connect to GitHub Actions
   - Automated image builds
   - Automated chart publishing

## 📚 Resources

- [Platform Operator Repo](https://github.com/NimbusProTch/PaaS-Platform)
- [ArgoCD Documentation](https://argo-cd.readthedocs.io/)
- [Gitea Documentation](https://docs.gitea.io/)
- [Kubernetes Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

## 📞 Support

For issues or questions:
- Create an issue on [GitHub](https://github.com/NimbusProTch/PaaS-Platform/issues)
- Check existing [documentation](./CLAUDE.md)

---

**Last Updated**: 2025-12-26
**Version**: 1.0.0
**Status**: Production Ready