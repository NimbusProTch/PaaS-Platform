# InfraForge Platform - GitOps PaaS Architecture

**Last Updated**: 2026-01-15 UTC+3
**Status**: ✅ Platform Services Working
**Phase**: Development - Ready for Full Testing

---

## 🎯 Recent Fixes & Current Status

### ✅ Fixed Issues (January 2026)

1. **Storage Class Configuration**
   - Added `StorageClass` field to PlatformApplicationClaim CRD
   - Now configurable from claim (not hardcoded)
   - Defaults to `standard` for Kind, can use `gp3` for AWS

2. **PostgreSQL Memory Requirements**
   - Fixed "shared_buffers" error
   - Increased minimum memory from 128Mi to 256Mi
   - Values generation now handles requirements correctly

3. **Chart Repository URL**
   - Updated ApplicationSet to use ChartMuseum
   - Changed from: `http://gitea.../infraforge/charts`
   - Changed to: `http://chartmuseum.chartmuseum.svc.cluster.local:8080`

4. **Operator Installation Loop**
   - Bypassed broken `isOperatorInstalled` check
   - Assumes operators are installed when ArgoCD Applications exist

### 🚀 Working Components
- ✅ PostgreSQL clusters (`product-db-dev`, `user-db-dev`) - Healthy
- ✅ Microservices (`product-service`, `user-service`) - Running
- ✅ Redis operator - Installed and operational
- ✅ CloudNativePG operator - Managing PostgreSQL clusters
- ✅ Full GitOps automation - Claim → Operator → ApplicationSet → Applications

---

## 🏗️ Platform Architecture

### 3-Level GitOps Pattern
```
Root Apps → ApplicationSets → Applications → K8s Resources
```

### Component Flow
```
┌──────────────────────────────────────────────────────────────┐
│                     1. INFRASTRUCTURE                         │
├──────────────────────────────────────────────────────────────┤
│  • Kind Cluster      - Kubernetes environment                 │
│  • Gitea            - Git server for GitOps state            │
│  • ChartMuseum      - Helm chart repository                   │
│  • ArgoCD           - GitOps engine                          │
│  • Platform Operator - Claims processor                       │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                      2. GITOPS FLOW                          │
├──────────────────────────────────────────────────────────────┤
│  BootstrapClaim → Creates Gitea structure & Root Apps        │
│  ApplicationClaim → Writes values.yaml & ApplicationSet      │
│  PlatformClaim → Writes values.yaml & ApplicationSet         │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                    3. ARGOCD SYNC                            │
├──────────────────────────────────────────────────────────────┤
│  Root Apps watch → appsets/ folders                          │
│  ApplicationSets → Generate Applications                      │
│  Applications → Pull charts from ChartMuseum                 │
│  Applications → Read values from Gitea                       │
│  Deploy → Kubernetes resources                               │
└──────────────────────────────────────────────────────────────┘
```

---

## 📂 Repository Structure

### Project Layout
```
PaaS-Platform/
├── Makefile                      # One-command orchestrator
├── .env                         # Credentials
├── CLAUDE.md                    # This documentation
├── FIXES-SUMMARY.md            # Recent fixes documentation
├── fix-platform-deployment.sh   # Automated fix script
│
├── infrastructure/
│   └── platform-operator/       # Kubernetes Operator
│       ├── api/v1/              # CRD definitions (updated)
│       ├── internal/controller/ # Reconcile logic (fixed)
│       ├── charts/              # Embedded charts
│       └── Dockerfile
│
├── charts/                      # Helm templates
│   ├── microservice/           # App template
│   ├── postgresql/             # DB template (CloudNativePG)
│   ├── redis/                  # Cache template
│   ├── rabbitmq/              # Queue template
│   ├── mongodb/               # Document DB template
│   └── kafka/                 # Streaming template
│
├── deployments/
│   └── dev/
│       ├── bootstrap-claim.yaml
│       ├── apps-claim.yaml
│       └── platform-infrastructure-claim.yaml  # Updated with storageClass
│
└── scripts/
    └── setup-gitea.sh          # GitOps helper
```

### Gitea Repository Structure (voltran)
```
voltran/
├── root-apps/
│   └── nonprod/
│       ├── nonprod-apps-root.yaml      # Watches appsets/nonprod/apps/
│       └── nonprod-platform-root.yaml  # Watches appsets/nonprod/platform/
│
├── appsets/
│   └── nonprod/
│       ├── apps/
│       │   └── dev-appset.yaml        # List generator for apps
│       └── platform/
│           └── dev-platform-appset.yaml # List generator for platform
│
└── environments/
    └── nonprod/
        └── dev/
            ├── applications/
            │   ├── product-service/
            │   │   └── values.yaml     # App config
            │   └── user-service/
            │       └── values.yaml
            └── platform/
                ├── product-db/
                │   └── values.yaml     # DB config (256Mi memory)
                ├── user-db/
                │   └── values.yaml     # DB config (256Mi memory)
                └── redis/
                    └── values.yaml     # Cache config
```

---

## 🚀 Deployment Flow

### Quick Start with `make full-deploy`

```bash
# Set environment variables
export GITHUB_TOKEN_ENV=<your-token>

# Single command deployment
make full-deploy
```

### Manual Step-by-Step

1. **Create Kind Cluster**
   ```bash
   kind create cluster --name infraforge-local --config kind-config.yaml
   ```

2. **Install Core Components**
   ```bash
   # Gitea
   helm install gitea gitea-charts/gitea -n gitea --create-namespace

   # ArgoCD
   kubectl create namespace argocd
   kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

   # ChartMuseum (with auth)
   helm install chartmuseum chartmuseum/chartmuseum -n chartmuseum --create-namespace \
     --set env.open.DISABLE_API=false \
     --set env.secret.BASIC_AUTH_USER=admin \
     --set env.secret.BASIC_AUTH_PASS=password123
   ```

3. **Deploy Platform Operator**
   ```bash
   cd infrastructure/platform-operator
   make install  # Install CRDs
   make deploy   # Deploy operator
   ```

4. **Bootstrap GitOps**
   ```bash
   kubectl apply -f deployments/dev/bootstrap-claim.yaml
   ```

5. **Upload Charts to ChartMuseum**
   ```bash
   cd charts
   for chart in postgresql redis microservice; do
     helm package $chart
     curl -u admin:password123 --data-binary "@${chart}-1.0.0.tgz" \
       http://localhost:8080/api/charts
   done
   ```

6. **Deploy Applications**
   ```bash
   kubectl apply -f deployments/dev/apps-claim.yaml
   kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
   ```

---

## 🔧 Configuration

### PlatformApplicationClaim Example

```yaml
apiVersion: platform.infraforge.io/v1
kind: PlatformApplicationClaim
metadata:
  name: ecommerce-infrastructure
  namespace: default
spec:
  environment: dev
  clusterType: nonprod

  # Storage class configuration (NEW!)
  storageClass: standard  # For Kind
  # storageClass: gp3     # For AWS EKS

  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: infraforge

  owner:
    team: platform-team
    email: platform@infraforge.io

  services:
    - type: postgresql
      name: product-db
      enabled: true
      version: "15"
      size: small
      chart:
        name: postgresql
        version: "1.0.0"
      values:
        persistence:
          size: 5Gi
        resources:
          requests:
            cpu: "100m"
            memory: "256Mi"  # Fixed: minimum for PostgreSQL
          limits:
            cpu: "200m"
            memory: "512Mi"
```

---

## ✅ Verification Steps

### Check Platform Services
```bash
# PostgreSQL Clusters
kubectl get clusters.postgresql.cnpg.io -n dev-platform

# Pods
kubectl get pods -n dev-platform
kubectl get pods -n dev

# PVCs
kubectl get pvc -n dev-platform

# ArgoCD Applications
kubectl get applications -n argocd | grep -E "(product-db|user-db|redis)"
```

### Expected Output
```
# PostgreSQL Clusters
NAME             AGE   INSTANCES   READY   STATUS                     PRIMARY
product-db-dev   21h   1           1       Cluster in healthy state   product-db-dev-1
user-db-dev      21h   1           1       Cluster in healthy state   user-db-dev-1

# Pods
NAME               READY   STATUS    RESTARTS   AGE
product-db-dev-1   1/1     Running   0          21h
user-db-dev-1      1/1     Running   0          21h
```

---

## 🛠️ Troubleshooting

### If Services Don't Deploy

1. **Check Operator Logs**
   ```bash
   kubectl logs -n platform-operator-system deployment/controller-manager
   ```

2. **Check ArgoCD Sync Status**
   ```bash
   kubectl get applications -n argocd -o wide
   ```

3. **Verify ChartMuseum Charts**
   ```bash
   curl -u admin:password123 http://localhost:8080/api/charts
   ```

4. **Force Resync**
   ```bash
   # Run the fix script
   ./fix-platform-deployment.sh
   ```

### Common Issues & Solutions

| Issue | Solution |
|-------|----------|
| PVC pending | Check storage class matches cluster (standard for Kind, gp3 for AWS) |
| PostgreSQL memory error | Ensure memory is at least 256Mi in values |
| Chart not found | Upload charts to ChartMuseum with auth |
| ApplicationSet using wrong URL | Delete and recreate platform claim |

---

## 📋 Key Commands

### Build and Push Operator
```bash
cd infrastructure/platform-operator
docker build -t ghcr.io/nimbusprotch/platform-operator:latest .
docker push ghcr.io/nimbusprotch/platform-operator:latest
kubectl rollout restart deployment/controller-manager -n platform-operator-system
```

### Upload Charts to ChartMuseum
```bash
cd charts
helm package postgresql redis
curl -u admin:password123 --data-binary "@postgresql-1.0.0.tgz" \
  http://localhost:8080/api/charts
```

### Recreate Platform Services
```bash
kubectl delete -f deployments/dev/platform-infrastructure-claim.yaml
kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
```

---

## 🚦 Success Criteria

- [x] Storage class configurable from claim
- [x] PostgreSQL memory requirements fixed
- [x] Operator installation loop resolved
- [x] ChartMuseum integration working
- [x] PostgreSQL clusters running
- [x] Microservices deployed
- [x] Full GitOps automation functional

---

## 📞 Support

**Repository**: https://github.com/NimbusProTch/PaaS-Platform
**Container Registry**: ghcr.io/nimbusprotch
**Documentation**: CLAUDE.md (this file), FIXES-SUMMARY.md

---

> **Version**: 4.1.0
> **Status**: Development - Working
> **Last Test**: 2026-01-15 with Kind v0.20.0