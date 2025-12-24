# InfraForge Platform - Architecture Documentation

**Last Updated**: 2025-12-24 19:00 UTC+3
**Status**: 🔄 Redesigning to GitOps-Native with Gitea
**Phase**: Architecture Finalization

---

## 🎯 Architecture Overview

### Platform Philosophy
InfraForge is a **Kubernetes-native PaaS platform** that enables developers to deploy applications through simple YAML claims. The platform automatically provisions infrastructure, configures GitOps workflows, and manages deployments through ArgoCD.

### Core Principles
1. **Operator-First**: Platform Operator handles all complexity
2. **Git as Source of Truth**: Every manifest stored in Gitea
3. **Minimal Terraform**: Infrastructure only, no business logic
4. **Developer-Friendly**: Single claim deploys entire environments
5. **GitOps-Native**: ArgoCD syncs from Git, not in-memory configs

---

## 🏗️ System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         TERRAFORM (Infrastructure)                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│  │  EKS        │  │   Gitea     │  │   ArgoCD    │                 │
│  │  Cluster    │  │  (empty)    │  │  (empty)    │                 │
│  └─────────────┘  └─────────────┘  └─────────────┘                 │
│                                                                      │
│  ┌──────────────────────────────────────────────────┐               │
│  │  Platform Operator (Helm OCI)                    │               │
│  │  - Charts embedded in image                      │               │
│  │  - Gitea client built-in                         │               │
│  └──────────────────────────────────────────────────┘               │
│                                                                      │
│  kubectl apply -f bootstrap-claim.yaml  ← Trigger                   │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                     BOOTSTRAP PHASE (Operator)                      │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ BootstrapClaim Reconciler                                      │ │
│  │  1. Create Gitea repos (charts, platform-charts, voltran)     │ │
│  │  2. Push embedded charts → Gitea                              │ │
│  │  3. Generate voltran folder structure                         │ │
│  │  4. Generate & push ArgoCD root apps                          │ │
│  │  5. Deploy root apps to ArgoCD                                │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  Status: Bootstrapped ✅                                            │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                   APPLICATION DEPLOYMENT PHASE                      │
│                                                                      │
│  Developer: kubectl apply -f dev-claim.yaml                         │
│             kubectl apply -f dev-platform-claim.yaml                │
│                              ↓                                      │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ ApplicationClaim Reconciler                                    │ │
│  │  1. Fetch GitHub package metadata (ghcr.io)                    │ │
│  │  2. Generate values.yaml                                       │ │
│  │  3. Push → Gitea: voltran/environments/.../values.yaml         │ │
│  │  4. Generate ApplicationSet YAML                               │ │
│  │  5. Push → Gitea: voltran/appsets/.../dev-appset.yaml          │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                              ↓                                      │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ PlatformClaim Reconciler (Postgres, RabbitMQ, Redis)          │ │
│  │  1. Generate platform service values.yaml                      │ │
│  │  2. Push → Gitea: voltran/environments/.../platform/           │ │
│  │  3. Generate platform ApplicationSet YAML                      │ │
│  │  4. Push → Gitea: voltran/appsets/.../platform-appset.yaml     │ │
│  └────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                     ARGOCD SYNC (Automated)                         │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ Root App: nonprod-apps → appsets/nonprod/apps/                │ │
│  │   ↓                                                            │ │
│  │ ApplicationSet: dev-appset.yaml                                │ │
│  │   ↓ (Git generator: environments/nonprod/dev/applications/*)  │ │
│  │ Applications:                                                  │ │
│  │   - dev-ecommerce-platform                                     │ │
│  │   - dev-user-service                                           │ │
│  │     ↓ (Pull chart from gitea/charts/, values from voltran/)   │ │
│  │   Deployed to Kubernetes! ✅                                   │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ Root App: nonprod-platform → appsets/nonprod/platform/        │ │
│  │   ↓                                                            │ │
│  │ ApplicationSet: dev-platform-appset.yaml                       │ │
│  │   ↓ (Git generator: environments/nonprod/dev/platform/*)      │ │
│  │ Applications:                                                  │ │
│  │   - dev-platform-postgres                                      │ │
│  │   - dev-platform-rabbitmq                                      │ │
│  │     ↓ (Pull chart from gitea/platform-charts/)                │ │
│  │   Deployed to platform-services namespace! ✅                  │ │
│  └────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 📂 Gitea Repository Structure

```
Gitea Organization: platform
│
├── 📦 charts/                         (Application Helm Charts)
│   ├── ecommerce-platform/
│   │   ├── Chart.yaml
│   │   ├── values.yaml                (base defaults)
│   │   └── templates/
│   │       ├── deployment.yaml
│   │       ├── service.yaml
│   │       └── ingress.yaml
│   ├── user-service/
│   ├── product-service/
│   └── order-service/
│
├── 📦 platform-charts/                (Platform Services - Postgres, Redis, etc)
│   ├── postgres/
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   ├── rabbitmq/
│   ├── redis/
│   └── kafka/
│
└── 📦 voltran/                        (GitOps Configuration Repository)
    ├── root-apps/                     🔥 Terraform creates, Operator populates
    │   ├── nonprod/
    │   │   ├── apps-rootapp.yaml
    │   │   └── platform-rootapp.yaml
    │   └── prod/
    │       ├── apps-rootapp.yaml
    │       └── platform-rootapp.yaml
    │
    ├── appsets/                       🔥 Operator creates dynamically
    │   ├── nonprod/
    │   │   ├── apps/
    │   │   │   ├── dev-appset.yaml       (generated by ApplicationClaim)
    │   │   │   ├── qa-appset.yaml
    │   │   │   └── sandbox-appset.yaml
    │   │   └── platform/
    │   │       ├── dev-platform-appset.yaml  (generated by PlatformClaim)
    │   │       ├── qa-platform-appset.yaml
    │   │       └── sandbox-platform-appset.yaml
    │   └── prod/
    │       ├── apps/
    │       │   ├── prod-appset.yaml
    │       │   └── stage-appset.yaml
    │       └── platform/
    │           ├── prod-platform-appset.yaml
    │           └── stage-platform-appset.yaml
    │
    └── environments/                  🔥 Operator creates values.yaml per app
        ├── nonprod/
        │   ├── dev/
        │   │   ├── applications/
        │   │   │   ├── ecommerce-platform/
        │   │   │   │   └── values.yaml      (ApplicationClaim → Operator generates)
        │   │   │   ├── user-service/
        │   │   │   │   └── values.yaml
        │   │   │   └── order-service/
        │   │   │       └── values.yaml
        │   │   └── platform/
        │   │       ├── postgres/
        │   │       │   └── values.yaml      (PlatformClaim → Operator generates)
        │   │       ├── rabbitmq/
        │   │       │   └── values.yaml
        │   │       └── redis/
        │   │           └── values.yaml
        │   ├── qa/
        │   │   ├── applications/
        │   │   └── platform/
        │   └── sandbox/
        │       ├── applications/
        │       └── platform/
        │
        └── prod/
            ├── prod/
            │   ├── applications/
            │   └── platform/
            └── stage/
                ├── applications/
                └── platform/
```

**📌 Structure Rules (Enforced by Operator)**:
- ✅ Fixed structure, no deviations allowed
- ✅ Operator generates all paths dynamically based on claims
- ✅ Git = Single Source of Truth (no ConfigMaps)
- ✅ Multi-cluster ready (same Git, different clusters)

---

## 🔧 Component Responsibilities

### 1. Terraform (Infrastructure Only)
```hcl
# Responsibilities:
- Deploy EKS cluster
- Deploy Gitea (empty)
- Deploy ArgoCD (empty)
- Deploy Platform Operator (from OCI Helm registry)
- Deploy BootstrapClaim (trigger operator)
- Deploy InfrastructureClaim (namespace setup)
- DONE! No Git operations, no kubectl apply loops

# Lines of Code: ~100 (previously 300+)
```

### 2. Platform Operator (All Intelligence)
```go
// Responsibilities:
1. Bootstrap:
   - Create Gitea repos
   - Push embedded charts
   - Generate folder structure
   - Create & deploy ArgoCD root apps

2. ApplicationClaim:
   - Fetch GitHub package metadata (ghcr.io)
   - Generate values.yaml
   - Push to Git: voltran/environments/.../applications/*/values.yaml
   - Generate ApplicationSet YAML
   - Push to Git: voltran/appsets/.../apps/*-appset.yaml

3. PlatformClaim:
   - Generate platform service values.yaml
   - Push to Git: voltran/environments/.../platform/*/values.yaml
   - Generate platform ApplicationSet YAML
   - Push to Git: voltran/appsets/.../platform/*-platform-appset.yaml

// Key Features:
- Git client built-in (go-git library)
- GitHub OCI package integration
- Idempotent reconciliation
- No ConfigMaps (Git only)
```

### 3. ArgoCD (Deployment Engine)
```yaml
# Responsibilities:
- Watch Gitea: voltran/appsets/*
- Generate Applications from ApplicationSets
- Pull Helm charts from Gitea
- Apply to Kubernetes
- Health checks & sync status

# No manual configuration needed
```

---

## 📋 Claim Specifications

### BootstrapClaim (One-time, Per Cluster)
```yaml
apiVersion: platform.infraforge.io/v1alpha1
kind: BootstrapClaim
metadata:
  name: platform-bootstrap
  namespace: platform-system
spec:
  gitea:
    url: http://gitea-http.gitea.svc:3000
    organization: platform

  clusters:
    - type: nonprod
      environments: [dev, qa, sandbox]
    - type: prod
      environments: [prod, stage]
```

**Operator Actions:**
1. Create repos: `charts`, `platform-charts`, `voltran`
2. Push embedded `/charts` → `platform/charts`
3. Push embedded `/platform-charts` → `platform/platform-charts`
4. Create folder structure in `voltran`
5. Generate & push root apps
6. Deploy root apps to ArgoCD

---

### ApplicationClaim (One Per Environment)
```yaml
apiVersion: platform.infraforge.io/v1alpha1
kind: ApplicationClaim
metadata:
  name: dev-apps
  namespace: dev
spec:
  clusterType: nonprod
  environment: dev

  applications:
    - name: ecommerce-platform
      chart:
        name: ecommerce-platform
        source: embedded  # Use gitea/platform/charts/
      image:
        repository: ghcr.io/infraforge/ecommerce-platform
        tag: v1.2.3
      values:
        replicas: 2
        ingress:
          enabled: true
          host: ecommerce-dev.example.com

    - name: user-service
      chart:
        name: user-service
        source: embedded
      image:
        repository: ghcr.io/infraforge/user-service
        tag: latest
      values:
        replicas: 1
```

**Operator Actions (per app):**
1. Fetch GitHub package metadata (digest, tags)
2. Generate `values.yaml`:
   ```yaml
   # voltran/environments/nonprod/dev/applications/ecommerce-platform/values.yaml
   image:
     repository: ghcr.io/infraforge/ecommerce-platform
     tag: v1.2.3
     pullPolicy: IfNotPresent
   replicas: 2
   ingress:
     enabled: true
     host: ecommerce-dev.example.com
   ```
3. Git commit & push
4. Generate `dev-appset.yaml`:
   ```yaml
   # voltran/appsets/nonprod/apps/dev-appset.yaml
   apiVersion: argoproj.io/v1alpha1
   kind: ApplicationSet
   metadata:
     name: dev-apps
   spec:
     generators:
       - git:
           repoURL: http://gitea.gitea.svc:3000/platform/voltran
           revision: main
           directories:
             - path: environments/nonprod/dev/applications/*
     template:
       metadata:
         name: 'dev-{{path.basename}}'
       spec:
         source:
           repoURL: http://gitea.gitea.svc:3000/platform/charts
           path: '{{path.basename}}'
           helm:
             valueFiles:
               - http://gitea.gitea.svc:3000/platform/voltran/raw/branch/main/environments/nonprod/dev/applications/{{path.basename}}/values.yaml
   ```
5. Git commit & push

---

### PlatformClaim (One Per Environment)
```yaml
apiVersion: platform.infraforge.io/v1alpha1
kind: PlatformClaim
metadata:
  name: dev-platform
  namespace: platform-services
spec:
  clusterType: nonprod
  environment: dev

  services:
    - name: postgres
      type: internal  # Use Helm chart (not RDS)
      values:
        primary:
          persistence:
            size: 10Gi
            storageClass: gp3
        auth:
          database: ecommerce
          username: admin

    - name: rabbitmq
      type: internal
      values:
        replicaCount: 1
        persistence:
          size: 8Gi
```

**Operator Actions (per service):**
1. Generate `values.yaml` for platform service
2. Push to `voltran/environments/nonprod/dev/platform/postgres/values.yaml`
3. Generate `dev-platform-appset.yaml`
4. Push to `voltran/appsets/nonprod/platform/dev-platform-appset.yaml`

---

## 📊 Execution Timeline

```
T+0min:  terraform apply started
T+5min:  EKS cluster ready ✅
T+7min:  Gitea deployed (empty) ✅
T+8min:  ArgoCD deployed (empty) ✅
T+9min:  Platform Operator deployed (from OCI Helm) ✅

T+10min: BootstrapClaim deployed
         Operator detects:
           → Create Gitea repos ✅
           → Push charts ✅
           → Create voltran structure ✅
           → Generate & push root apps ✅
           → Deploy root apps to ArgoCD ✅
         Status: Bootstrapped ✅

T+15min: InfrastructureClaim deployed
         Operator: Namespace configs created

T+16min: ApplicationClaim (dev-apps) deployed
         Operator:
           → Fetch GitHub packages ✅
           → Generate values.yaml ✅
           → Push to Git ✅
           → Generate ApplicationSet ✅
           → Push to Git ✅

T+17min: ArgoCD sync starts
         Root App → ApplicationSet → Applications
         Applications deploy from:
           - Chart: gitea/platform/charts/...
           - Values: gitea/platform/voltran/...
           - Image: ghcr.io/infraforge/...

T+18min: DEPLOYED! 🚀

terraform apply completed!
```

---

## 🔬 Key Design Decisions

### ❌ What We Removed
- **ConfigMaps**: Git is source of truth
- **Terraform Git Operations**: Operator handles all Git
- **ChartMuseum**: Using Gitea for charts
- **Manual kubectl loops**: One claim per environment

### ✅ What We Gained
- **Single Source of Truth**: All manifests in Git
- **Audit Trail**: Git history tracks all changes
- **Multi-Cluster Ready**: Share Git URL across clusters
- **Operator-First**: Terraform just provisions infrastructure
- **Clean Separation**: Infrastructure (Terraform) vs Logic (Operator)

---

## 📁 Project Structure

```
PaaS-Platform/
├── charts/                        🔥 Embedded in operator image
│   ├── ecommerce-platform/
│   ├── user-service/
│   └── product-service/
│
├── platform-charts/               🔥 Embedded in operator image
│   ├── postgres/
│   ├── rabbitmq/
│   └── redis/
│
├── infrastructure/
│   ├── aws/                       (Terraform - minimal)
│   │   ├── main.tf
│   │   ├── vpc.tf
│   │   ├── eks.tf
│   │   ├── gitea.tf               🔥 NEW
│   │   ├── argocd.tf
│   │   └── gitea-bootstrap.tf     🔥 NEW (minimal)
│   │
│   └── platform-operator/         (Operator code)
│       ├── api/v1alpha1/
│       │   ├── bootstrapclaim_types.go      🔥 NEW
│       │   ├── applicationclaim_types.go
│       │   └── platformclaim_types.go       🔥 NEW
│       ├── internal/controller/
│       │   ├── bootstrap_controller.go      🔥 NEW
│       │   ├── applicationclaim_controller.go
│       │   └── platformclaim_controller.go  🔥 NEW
│       ├── pkg/
│       │   ├── gitea/              🔥 NEW (Git client)
│       │   └── github/             🔥 NEW (OCI package client)
│       ├── Dockerfile              (Charts embedded)
│       └── Makefile
│
└── deployments/                   (Example claims)
    ├── bootstrap-claim.yaml       🔥 NEW
    ├── dev/
    │   ├── dev-apps-claim.yaml
    │   └── dev-platform-claim.yaml
    └── prod/
        ├── prod-apps-claim.yaml
        └── prod-platform-claim.yaml
```

---

## 🚀 Next Steps

### Phase 1: Operator Development
1. ✅ Define CRDs (BootstrapClaim, ApplicationClaim, PlatformClaim)
2. ✅ Implement Bootstrap Controller
3. ✅ Implement ApplicationClaim Controller
4. ✅ Implement PlatformClaim Controller
5. ✅ Add Gitea client library
6. ✅ Add GitHub OCI package client
7. ✅ Build & test locally (Orbstack + Gitea)

### Phase 2: Terraform Integration
1. Deploy Gitea via Helm
2. Deploy Operator via OCI Helm chart
3. Deploy BootstrapClaim
4. Validate end-to-end flow

### Phase 3: Production Hardening
1. Error handling & retries
2. Status conditions & events
3. Webhook validations
4. RBAC policies
5. Multi-cluster testing

---

## 📞 Support & Contributing

**Repository**: https://github.com/infraforge/PaaS-Platform
**Status**: Active Development
**License**: MIT

---

**Last Updated**: 2025-12-24 19:00 UTC+3
**Next Review**: After Bootstrap Controller implementation
