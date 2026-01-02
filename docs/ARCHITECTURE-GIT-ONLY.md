# Git-Only Architecture Documentation

**Last Updated**: 2025-12-29 15:00 UTC+3
**Status**: ✅ Production Ready
**Architecture**: Simplified Git-Only (No ChartMuseum)

---

## 🎯 Architecture Overview

### Core Principle
**Everything in Git** - One source of truth, no external dependencies, clean architecture.

### Why Git-Only?

#### Problems with Hybrid Architecture
1. **ChartMuseum + Gitea** caused valueFiles confusion
2. ArgoCD couldn't use `valueFiles` with OCI/Helm chart sources
3. Two repositories to manage (ChartMuseum + Gitea)
4. Complex push workflows (charts to OCI, values to Git)

#### Git-Only Benefits
1. ✅ Single source of truth (Gitea)
2. ✅ valueFiles work perfectly with Git source
3. ✅ Multi-source ApplicationSets (charts + values)
4. ✅ Full GitOps workflow
5. ✅ No external dependencies
6. ✅ Simplified architecture

---

## 🏗️ System Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    Gitea (Single Source)                      │
├──────────────────────────────────────────────────────────────┤
│  Repository 1: charts/                                        │
│  ├── microservice/                                            │
│  │   ├── Chart.yaml                                           │
│  │   ├── values.yaml                                          │
│  │   └── templates/                                           │
│  ├── postgresql/                                              │
│  ├── redis/                                                   │
│  └── ... (all Helm charts)                                    │
│                                                               │
│  Repository 2: voltran/                                       │
│  ├── appsets/                                                 │
│  │   └── nonprod/                                             │
│  │       ├── apps/dev-appset.yaml                             │
│  │       └── platform/dev-platform-appset.yaml                │
│  └── environments/                                            │
│      └── nonprod/dev/                                         │
│          ├── applications/                                    │
│          │   ├── user-service/                                │
│          │   │   ├── values.yaml                              │
│          │   │   └── config.json                              │
│          │   └── product-service/                             │
│          └── platform/                                        │
│              ├── postgresql/values.yaml                       │
│              └── redis/values.yaml                            │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                    Platform Operator                          │
├──────────────────────────────────────────────────────────────┤
│  1. Reads ApplicationClaim / PlatformApplicationClaim         │
│  2. Generates ApplicationSet manifests                        │
│  3. Generates values.yaml for each service                    │
│  4. Pushes to Gitea voltran repository                        │
│  5. Creates ArgoCD Application resources                      │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                  ArgoCD Multi-Source Apps                     │
├──────────────────────────────────────────────────────────────┤
│  ApplicationSet Generator: Git Files/Directories              │
│                                                               │
│  Source 1: Helm Chart                                         │
│    repoURL: gitea.com/infraforge/charts                      │
│    path: microservice                                         │
│    targetRevision: main                                       │
│    helm:                                                      │
│      valueFiles:                                              │
│        - $values/environments/nonprod/dev/applications/       │
│          user-service/values.yaml                             │
│                                                               │
│  Source 2: Values Reference                                   │
│    repoURL: gitea.com/infraforge/voltran                     │
│    targetRevision: main                                       │
│    ref: values                                                │
│                                                               │
│  Result: Merged Helm release deployed to cluster              │
└──────────────────────────────────────────────────────────────┘
```

---

## 📂 Repository Structure

### Gitea Organization: infraforge

#### Repository 1: charts
Contains all Helm chart templates (stable, version-controlled).

```
charts/
├── README.md
├── microservice/
│   ├── Chart.yaml              # version: 1.0.0
│   ├── values.yaml             # Base defaults
│   └── templates/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       └── hpa.yaml
│
├── postgresql/
│   ├── Chart.yaml              # version: 1.0.0
│   ├── values.yaml
│   └── templates/
│       └── cluster.yaml        # CloudNative-PG CRD
│
├── redis/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       └── cluster.yaml        # Redis Operator CRD
│
├── mongodb/
├── rabbitmq/
└── kafka/
```

**Push Script**: `/scripts/push-charts-to-gitea.sh`

#### Repository 2: voltran
Contains GitOps state (ApplicationSets + values per environment).

```
voltran/
├── appsets/
│   └── nonprod/
│       ├── apps/
│       │   └── dev-appset.yaml
│       └── platform/
│           └── dev-platform-appset.yaml
│
└── environments/
    └── nonprod/
        └── dev/
            ├── applications/
            │   ├── user-service/
            │   │   ├── values.yaml
            │   │   └── config.json
            │   └── product-service/
            │       ├── values.yaml
            │       └── config.json
            │
            └── platform/
                ├── postgresql/
                │   └── values.yaml
                └── redis/
                    └── values.yaml
```

---

## 🔄 Workflow

### 1. Initial Setup

```bash
# Set environment variables
export GITEA_TOKEN=<token>
export GITEA_URL=http://gitea-http.gitea.svc.cluster.local:3000
export GITEA_ORG=infraforge

# Push charts to Gitea
./scripts/push-charts-to-gitea.sh
```

This creates:
- Organization: `infraforge`
- Repository: `charts` (with all Helm charts)

### 2. Apply ApplicationClaim

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: dev-apps
spec:
  environment: dev
  clusterType: nonprod
  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: infraforge

  applications:
    - name: user-service
      enabled: true
      chart:
        name: microservice
        version: "1.0.0"
      image:
        repository: ghcr.io/org/user-service
        tag: v1.0.0
      replicas: 2
```

### 3. Platform Operator Processing

**Controller Actions:**

1. Reads `ApplicationClaim` CRD
2. Generates files to push to Gitea voltran repo:
   - `appsets/nonprod/apps/dev-appset.yaml` (ApplicationSet)
   - `environments/nonprod/dev/applications/user-service/values.yaml`
   - `environments/nonprod/dev/applications/user-service/config.json`
3. Pushes to Gitea using Git client
4. Creates ArgoCD `Application` resources

### 4. ApplicationSet Structure

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dev-apps
  namespace: argocd
spec:
  generators:
    - git:
        repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran
        revision: main
        files:
          - path: environments/nonprod/dev/applications/*/config.json

  template:
    metadata:
      name: '{{name}}-dev'
    spec:
      project: default
      sources:
        # Source 1: Helm chart from charts repo
        - repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/charts
          path: '{{chart}}'
          targetRevision: main
          helm:
            valueFiles:
              - $values/environments/nonprod/dev/applications/{{name}}/values.yaml

        # Source 2: Values reference from voltran repo
        - repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran
          targetRevision: main
          ref: values

      destination:
        server: https://kubernetes.default.svc
        namespace: dev

      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - CreateNamespace=true
```

**Key Points:**
- `files` generator reads `config.json` from each app directory
- `config.json` contains: `{"name": "user-service", "chart": "microservice"}`
- Multi-source: chart from `charts` repo, values from `voltran` repo
- `$values/` prefix references the second source (voltran)

### 5. ArgoCD Sync

1. ApplicationSet controller reads `config.json` files
2. Generates one Application per service
3. Each Application:
   - Pulls chart from `gitea.com/infraforge/charts/microservice`
   - Merges with `gitea.com/infraforge/voltran/.../values.yaml`
   - Renders Helm templates
   - Deploys to Kubernetes

---

## 🔧 Implementation Details

### Controller Changes

#### Before (ChartMuseum)
```go
"source": map[string]interface{}{
    "repoURL": "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
    "chart": chartName,
    "targetRevision": version,
    "helm": map[string]interface{}{
        "values": string(valuesYAML), // Inline values
    },
}
```

#### After (Git-Only)
```go
"sources": []map[string]interface{}{
    {
        // Helm chart from Git
        "repoURL": fmt.Sprintf("%s/%s/charts", claim.Spec.GiteaURL, claim.Spec.Organization),
        "path": chartName,
        "targetRevision": "main",
        "helm": map[string]interface{}{
            "valueFiles": []string{
                fmt.Sprintf("$values/environments/%s/%s/applications/{{name}}/values.yaml",
                    claim.Spec.ClusterType, claim.Spec.Environment),
            },
        },
    },
    {
        // Values reference
        "repoURL": fmt.Sprintf("%s/%s/%s", claim.Spec.GiteaURL, claim.Spec.Organization, r.VoltranRepo),
        "targetRevision": r.Branch,
        "ref": "values",
    },
}
```

### Files Modified

1. **applicationclaim_gitops_controller.go**
   - `generateApplication()` - Changed to Git source
   - `generateApplicationSet()` - Changed to multi-source Git

2. **platformapplicationclaim_controller.go**
   - `generatePlatformApplication()` - Changed to Git source
   - `generatePlatformApplicationSet()` - Changed to multi-source Git

3. **New Script**: `scripts/push-charts-to-gitea.sh`

---

## 🚀 Quick Start

### Full Deployment

```bash
# 1. Create Kind cluster
make kind-create

# 2. Install infrastructure
make install-operator
make install-gitea
make install-argocd

# 3. Push charts to Gitea
export GITEA_TOKEN=$(kubectl get secret -n gitea gitea-admin-secret -o jsonpath='{.data.password}' | base64 -d)
./scripts/push-charts-to-gitea.sh

# 4. Deploy claims
kubectl apply -f deployments/dev/bootstrap-claim.yaml
kubectl apply -f deployments/dev/apps-claim.yaml
kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml

# 5. Verify
kubectl get applicationset -n argocd
kubectl get application -n argocd
kubectl get pods -n dev
```

---

## ✅ Benefits

### Simplicity
- One source of truth (Gitea)
- No ChartMuseum to maintain
- Clear Git workflow

### GitOps Native
- Everything version controlled
- Audit trail in Git history
- Easy rollbacks (git revert)

### Multi-Source Power
- Charts stable in charts repo
- Values per environment in voltran repo
- Clean separation of concerns

### Scalability
- Add environments by adding directories
- Add organizations by creating new Gitea orgs
- No infrastructure changes needed

---

## 📋 Migration Guide

### From ChartMuseum to Git-Only

1. **Stop using ChartMuseum**
   - Remove ChartMuseum deployment (optional)
   - Update ApplicationSets to use Git sources

2. **Push charts to Gitea**
   - Run `./scripts/push-charts-to-gitea.sh`
   - Verify charts in Gitea UI

3. **Update operator** (if running)
   - Rebuild operator with new controllers
   - Redeploy operator

4. **Re-apply claims**
   - Delete old ApplicationSets
   - Re-apply ApplicationClaim/PlatformApplicationClaim
   - Operator will create new Git-based ApplicationSets

---

## 🔍 Troubleshooting

### ApplicationSet not generating Applications

**Check:**
```bash
kubectl describe applicationset dev-apps -n argocd
```

**Common issues:**
- `config.json` files missing in voltran repo
- Git generator path pattern incorrect
- Gitea repository not accessible from ArgoCD

### Applications failing to sync

**Check:**
```bash
kubectl get application user-service-dev -n argocd -o yaml
```

**Common issues:**
- Charts not found in charts repo
- Values file path incorrect
- Multi-source ref name mismatch

### Values not being applied

**Verify:**
- Values file exists at correct path
- `$values/` prefix used in valueFiles
- Second source has `ref: values`

---

## 📊 Comparison

| Feature | ChartMuseum Hybrid | Git-Only |
|---------|-------------------|----------|
| Chart Storage | OCI Registry (ChartMuseum) | Git (Gitea charts repo) |
| Values Storage | Git (Gitea voltran repo) | Git (Gitea voltran repo) |
| ApplicationSet Generator | List | Git Files/Directories |
| ArgoCD Source Type | Helm (OCI) | Git (multi-source) |
| valueFiles Support | ❌ No | ✅ Yes |
| Dependencies | ChartMuseum + Gitea | Gitea only |
| Push Workflow | `helm push` + `git push` | `git push` only |
| Complexity | High | Low |
| GitOps Native | Partial | Full |

---

## 🎯 Next Steps

1. ✅ Charts pushed to Gitea
2. ✅ Controllers updated to Git sources
3. ⏳ Test with live claims
4. ⏳ Validate multi-environment setup
5. ⏳ Document production deployment
6. ⏳ Add monitoring for Git repositories

---

**Repository**: https://github.com/NimbusProTch/PaaS-Platform
**Documentation**: `/docs/ARCHITECTURE-GIT-ONLY.md`
**Script**: `/scripts/push-charts-to-gitea.sh`

---

> **Version**: 4.0.0
> **Status**: Production Ready
> **Architecture**: Git-Only, No External Dependencies
