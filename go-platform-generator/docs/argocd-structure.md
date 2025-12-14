# ArgoCD Application Structure

## Overview

The platform uses a hierarchical ArgoCD structure to manage both platform and business applications.

## Structure

```
argocd/
├── root-app.yaml                 # Root application
├── platform/
│   ├── appset-platform.yaml      # Platform apps ApplicationSet
│   └── kratix-claims/            # Kratix-generated apps
│       ├── redis-*.yaml
│       ├── nginx-*.yaml
│       └── ...
└── business/
    ├── appset-business.yaml      # Business apps ApplicationSet
    └── apps/
        ├── app1/
        └── app2/
```

## Application Categories

### 1. Platform Applications
- **Managed by**: Kratix claims
- **Examples**: Redis, Nginx, PostgreSQL, Keycloak
- **Namespace pattern**: `{component}-{tenant}-{env}`
- **Labels**:
  - `app.kubernetes.io/managed-by: kratix`
  - `platform.paas/type: platform`

### 2. Business Applications
- **Managed by**: Direct ArgoCD applications
- **Examples**: Custom microservices, APIs
- **Namespace pattern**: `{appname}-{env}`
- **Labels**:
  - `app.kubernetes.io/managed-by: argocd`
  - `platform.paas/type: business`

## Root Application

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/gaskin/paas-platform
    path: argocd
    targetRevision: main
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

## Platform ApplicationSet

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: platform-apps
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: https://github.com/gaskin/paas-platform
      revision: main
      directories:
      - path: argocd/platform/kratix-claims/*
  template:
    metadata:
      name: '{{path.basename}}'
    spec:
      project: platform
      source:
        repoURL: https://github.com/gaskin/paas-platform
        path: '{{path}}'
      destination:
        server: https://kubernetes.default.svc
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

## GitOps Flow

1. **Infrastructure Creation** (Terragrunt)
   - Creates K8s cluster
   - Installs ArgoCD
   - Installs Kratix

2. **Root App Deployment**
   - Deploys ApplicationSets
   - Sets up projects

3. **Platform Stack Claims**
   - User creates PlatformStack claim
   - Kratix processes and generates ArgoCD apps
   - Apps pushed to Git repository
   - ApplicationSet picks up new apps

4. **Business Apps**
   - Developers push app manifests
   - Business ApplicationSet deploys them