# InfraForge Platform - GitOps PaaS Architecture

**Last Updated**: 2025-12-29 UTC+3
**Status**: 🔧 ApplicationSet Fix in Progress
**Phase**: Development - GitOps Flow Fix

---

## 🎯 Current Issue & Solution

### ❌ Problem
- ApplicationSets use List Generator with `{{values}}` placeholder
- ArgoCD cannot parse `{{values}}` string interpolation
- Applications not being generated from ApplicationSets

### ✅ Solution
- Switch from List Generator to Git Directories Generator
- Use `valueFiles` to read from Gitea instead of inline values
- Simplify ApplicationSet structure

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
│  • ChartMuseum      - HTTP Helm repository                   │
│  • ArgoCD           - GitOps engine (v3.2.3)                 │
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
│
├── infrastructure/
│   └── platform-operator/       # Kubernetes Operator
│       ├── api/v1/              # CRD definitions
│       ├── internal/controller/ # Reconcile logic
│       └── Dockerfile
│
├── charts/                      # Helm templates
│   ├── microservice/           # App template
│   ├── postgresql/             # DB template
│   └── redis/                  # Cache template
│
├── deployments/
│   └── dev/
│       ├── bootstrap-claim.yaml
│       ├── apps-claim.yaml
│       └── platform-infrastructure-claim.yaml
│
└── scripts/
    └── setup-gitea.sh          # GitOps helper
```

### Gitea Repository (voltran)
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
│       │   └── dev-appset.yaml        # Git generator for apps
│       └── platform/
│           └── dev-platform-appset.yaml # Git generator for platform
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
                │   └── values.yaml     # DB config
                ├── user-db/
                │   └── values.yaml
                └── redis/
                    └── values.yaml     # Cache config
```

---

## 🚀 Deployment Flow

### `make full-deploy` Steps

1. **Create Kind Cluster**
   ```bash
   kind create cluster --name infraforge-local
   ```

2. **Install Gitea**
   ```bash
   helm install gitea gitea-charts/gitea
   ```

3. **Install ArgoCD**
   ```bash
   kubectl apply -f argocd-install.yaml
   ```

4. **Install ChartMuseum**
   ```bash
   helm install chartmuseum chartmuseum/chartmuseum
   ```

5. **Deploy Platform Operator**
   ```bash
   kubectl apply -k infrastructure/platform-operator/config
   ```

6. **Bootstrap GitOps**
   ```bash
   kubectl apply -f deployments/dev/bootstrap-claim.yaml
   ```
   - Creates Gitea repos
   - Creates folder structure
   - Creates Root Applications

7. **Upload Charts**
   ```bash
   helm package charts/* && curl to ChartMuseum
   ```

8. **Deploy Applications**
   ```bash
   kubectl apply -f deployments/dev/apps-claim.yaml
   kubectl apply -f deployments/dev/platform-claim.yaml
   ```

---

## 🔧 ApplicationSet Fix (Current Work)

### Before (Broken - List Generator)
```yaml
generators:
- list:
    elements:
    - name: product-service
      values: "{{values}}"  # ❌ Doesn't work
```

### After (Fixed - Git Directories)
```yaml
generators:
- git:
    repoURL: http://gitea.../voltran
    directories:
    - path: environments/nonprod/dev/applications/*
template:
  spec:
    helm:
      valueFiles:
      - '{{path}}/values.yaml'  # ✅ Works
```

---

## ✅ Task List

- [x] Create infrastructure setup
- [x] Deploy Platform Operator
- [x] Setup Gitea GitOps structure
- [x] Create ChartMuseum repository
- [ ] Fix ApplicationSet generator (IN PROGRESS)
- [ ] Build and deploy fixed operator
- [ ] Test end-to-end flow
- [ ] Deploy sample microservices
- [ ] Verify pod health

---

## 🎯 Key Commands

### Quick Status Check
```bash
# Check system
kubectl get pods -n dev
kubectl get applications -n argocd
kubectl get applicationsets -n argocd

# Check operator logs
kubectl logs -n platform-operator-system deployment/controller-manager
```

### Rebuild Operator
```bash
# Build locally
cd infrastructure/platform-operator
docker build -t ghcr.io/nimbusprotch/platform-operator:latest .
docker push ghcr.io/nimbusprotch/platform-operator:latest

# Restart operator
kubectl rollout restart deployment/controller-manager -n platform-operator-system
```

### Clean Restart
```bash
# Delete broken ApplicationSets
kubectl delete applicationset dev-apps dev-platform -n argocd

# Re-apply claims
kubectl delete -f deployments/dev/
kubectl apply -f deployments/dev/
```

---

## 📋 Environment Variables

Create `.env` file with:
```bash
GITHUB_TOKEN_ENV=<your-github-token>
GITEA_ADMIN_USER=gitea_admin
GITEA_ADMIN_PASS=<generated-password>
```

---

## 🔄 Next Steps

1. **Immediate**: Fix ApplicationSet generator code
2. **Short-term**: Deploy and test fixed operator
3. **Mid-term**: Build actual microservice images
4. **Long-term**: Production deployment on cloud

---

**Repository**: https://github.com/NimbusProTch/PaaS-Platform
**Container Registry**: ghcr.io/nimbusprotch
**Documentation**: This file (CLAUDE.md)

---

> **Version**: 4.0.0-dev
> **Architecture**: GitOps-based, 3-Level Pattern
> **Status**: Fixing ApplicationSet Generation