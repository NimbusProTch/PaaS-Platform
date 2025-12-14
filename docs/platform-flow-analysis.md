# Platform Flow ve Multi-Environment Desteği

## 1. Mevcut Manifest Yapısı

```
manifests/
├── voltron/                    # Kratix'in yazdığı yer (GitStateStore path)
│   └── <tenant>-<env>/        # Namespace bazlı ayrım
│       ├── apps/<tenant>/<env>/<service>/
│       │   └── <service>-application.yaml   # ArgoCD Application
│       └── argocd/<tenant>/<env>/
│           └── services-appset.yaml         # ApplicationSet
├── operators/                  # Operatörler (şu an boş)
├── platform/                   # Platform components
└── argocd/                    # Root applications (şu an boş)
```

## 2. Multi-Environment Desteği Analizi

### Mevcut Durum:
- ✅ Tenant-env bazlı namespace isolation
- ✅ Her environment için ayrı folder
- ❌ Single cluster assumption
- ❌ Environment-based operator versioning yok

### Önerilen Yapı:
```
manifests/
└── voltron/
    ├── platform/                     # Platform-wide resources
    │   ├── operators/               # Shared operators
    │   │   ├── cloudnative-pg/
    │   │   ├── mongodb-operator/
    │   │   └── redis-operator/
    │   └── monitoring/              # Shared monitoring
    │
    ├── <tenant>-dev/               # Dev environment
    ├── <tenant>-test/              # Test environment  
    ├── <tenant>-uat/               # UAT environment
    └── <tenant>-prod/              # Prod environment
```

## 3. CloudNativePG Integration Plan

### Template Structure:
```
platform-templates/
└── postgresql/
    ├── operator/
    │   └── operator.yaml.tmpl      # Operator installation (once per cluster)
    ├── cluster/
    │   └── cluster.yaml.tmpl       # PostgreSQL cluster CR
    ├── backup/
    │   └── backup.yaml.tmpl        # Backup configuration
    └── profiles/
        ├── dev.yaml                # 1 instance, no backup
        ├── standard.yaml           # 3 instances, daily backup
        └── production.yaml         # 5 instances, continuous backup
```

### Operator Installation Strategy:
```yaml
# Option 1: Bootstrap Phase (Recommended)
# infrastructure/bootstrap/cnpg-operator.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cloudnative-pg
  namespace: infraforge-argocd
spec:
  project: platform
  source:
    repoURL: https://cloudnative-pg.github.io/charts
    chart: cloudnative-pg
    targetRevision: 0.20.0
  destination:
    server: https://kubernetes.default.svc
    namespace: cnpg-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
  # Wave 10 - Operators install first
  annotations:
    argocd.argoproj.io/sync-wave: "10"
```

## 4. Claim Standard Definition

### InfraForge CR Structure:
```yaml
apiVersion: platform.infraforge.io/v1
kind: InfraForge
metadata:
  name: my-app-stack
  namespace: default
spec:
  # Tenant information
  tenant: myteam              # Team/tenant identifier
  environment: dev            # dev/test/uat/prod
  
  # Platform resources (optional)
  platform:
    operators:
      postgresql: true        # Install if not exists
      mongodb: false
    infrastructure:
      monitoring: true        # Prometheus/Grafana per namespace
      logging: true          # Fluentbit/Loki per namespace
  
  # Application services
  services:
  - name: api-db             # Service instance name
    type: postgresql         # Service type
    profile: production      # Profile selection
    config:                  # Override profile defaults
      replicas: 3
      storage: 100Gi
      backup:
        enabled: true
        schedule: "0 */6 * * *"
      monitoring:
        enabled: true
        metrics:
        - pg_stat_statements
        - pg_stat_replication
  
  - name: cache
    type: redis
    profile: standard
    config:
      maxMemory: 4Gi
      persistence: true
```

## 5. Template Engine Flow

### Current Generator Flow:
```go
// go-platform-generator/pkg/pipeline/processor.go

1. Read InfraForge CR
2. For each service:
   a. Load profile (dev/standard/prod)
   b. Merge with config overrides
   c. Load templates from platform-templates/<type>/
   d. Render templates with merged values
   e. Generate ArgoCD Application
   f. Generate ApplicationSet

3. Output structure:
   <tenant>-<env>/
   ├── apps/<tenant>/<env>/<service>/
   │   └── <name>-application.yaml
   └── argocd/<tenant>/<env>/
       └── services-appset.yaml
```

### Template Variables Available:
```go
type TemplateData struct {
    // From InfraForge CR
    Tenant      string
    Environment string
    Service     ServiceSpec
    
    // Computed
    Namespace   string  // <tenant>-<env>
    ChartRepo   string  // From service definition
    ChartName   string
    
    // Merged values
    Values      map[string]interface{}  // Profile + overrides
}
```

## 6. Multi-Environment ArgoCD Structure

### Proposed App of Apps Pattern:
```yaml
# manifests/voltron/argocd/root-apps/nonprod-root.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: nonprod-root
  namespace: infraforge-argocd
spec:
  project: default
  source:
    repoURL: https://github.com/gaskin1/PaaS-Platform.git
    path: manifests/voltron/argocd/nonprod
    targetRevision: feature/kratix
  destination:
    server: https://kubernetes.default.svc
    namespace: infraforge-argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
---
# manifests/voltron/argocd/nonprod/platform-operators.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: platform-operators
spec:
  generators:
  - git:
      repoURL: https://github.com/gaskin1/PaaS-Platform.git
      revision: feature/kratix
      directories:
      - path: manifests/voltron/platform/operators/*
  template:
    metadata:
      name: '{{path.basename}}'
    spec:
      project: platform
      source:
        repoURL: https://github.com/gaskin1/PaaS-Platform.git
        targetRevision: feature/kratix
        path: '{{path}}'
      destination:
        server: https://kubernetes.default.svc
        namespace: '{{path.basename}}-system'
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
        - CreateNamespace=true
---
# manifests/voltron/argocd/nonprod/tenant-apps.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: tenant-applications
spec:
  generators:
  - git:
      repoURL: https://github.com/gaskin1/PaaS-Platform.git
      revision: feature/kratix
      directories:
      - path: manifests/voltron/*-dev/argocd/*/*
      - path: manifests/voltron/*-test/argocd/*/*
      - path: manifests/voltron/*-uat/argocd/*/*
  template:
    metadata:
      name: '{{path[2]}}-{{path[4]}}-{{path[5]}}'
    spec:
      project: '{{path[4]}}'  # Environment as project
      source:
        repoURL: https://github.com/gaskin1/PaaS-Platform.git
        targetRevision: feature/kratix
        path: '{{path}}'
      destination:
        server: https://kubernetes.default.svc
        namespace: infraforge-argocd
```

## 7. Critical Design Decisions

### A. Operator Lifecycle:
**Decision**: Bootstrap operators at platform level
- All operators installed once per cluster
- Versioning controlled by platform team
- CRs created per tenant/service

### B. Multi-Cluster vs Multi-Namespace:
**Current**: Single cluster, namespace isolation
**Future**: Multi-cluster with namespace isolation
```
Dev Cluster:   *-dev namespaces
Test Cluster:  *-test namespaces  
Prod Cluster:  *-uat, *-prod namespaces
```

### C. GitOps Repository Structure:
**Decision**: Single repo, path-based separation
- Simplified management
- Branch protection for prod paths
- CODEOWNERS for tenant isolation

## 8. Implementation Priority

1. **CloudNativePG Templates** (Today)
   - Create operator template
   - Create cluster templates
   - Add to service registry

2. **Multi-Env ApplicationSets** (Tomorrow)
   - Root app structure
   - Platform vs tenant separation
   - Environment-based projects

3. **Enhanced Generator** (This week)
   - Support operator CRs
   - Config override mechanism
   - Validation logic

4. **Documentation** (Continuous)
   - Template guide
   - Claim examples
   - Troubleshooting