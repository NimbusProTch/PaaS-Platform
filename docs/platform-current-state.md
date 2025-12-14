# Platform Current State & Multi-Environment Strategy

## 🎯 Mevcut Durum

### GitOps Flow:
```
1. Developer → InfraForge CR oluşturur
2. Kratix → manifests/voltron/<tenant>-<env>/ altına yazar
3. GitHub → Dosyalar otomatik push edilir
4. ArgoCD Bootstrap → argocd/**/*.yaml dosyalarını bulur
5. ApplicationSet → apps/ klasörlerini tarar
6. Application → Helm chart deploy eder
```

### Folder Structure:
```
manifests/voltron/
├── demo-dev/
│   ├── apps/demo/dev/nginx/
│   │   └── web-application.yaml     # ArgoCD Application
│   └── argocd/demo/dev/
│       └── services-appset.yaml      # ApplicationSet
├── demo-test/                        # Test environment (future)
├── demo-uat/                         # UAT environment (future)
└── demo-prod/                        # Prod environment (future)
```

## 🔧 Multi-Environment Support

### Current Implementation:
- ✅ Path-based separation ready
- ✅ Namespace isolation: <tenant>-<env>
- ✅ ApplicationSet per environment
- ❌ Single cluster assumption
- ❌ No environment-specific configurations

### Proposed Enhancement:

#### 1. Environment-Specific Projects:
```yaml
# infrastructure/argocd/argocd-projects.yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infraforge-dev
spec:
  destinations:
  - namespace: '*-dev'
    server: https://kubernetes.default.svc
  sourceRepos:
  - 'https://github.com/gaskin1/PaaS-Platform.git'
  clusterResourceWhitelist:
  - group: ''
    kind: Namespace
---
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infraforge-prod
spec:
  destinations:
  - namespace: '*-prod'
    server: https://kubernetes.default.svc
  - namespace: '*-uat'
    server: https://kubernetes.default.svc
  sourceRepos:
  - 'https://github.com/gaskin1/PaaS-Platform.git'
```

#### 2. Multi-Cluster Support (Future):
```yaml
# Kratix Destination per cluster
apiVersion: platform.kratix.io/v1alpha1
kind: Destination
metadata:
  name: dev-cluster
  labels:
    environment: dev
    infraforge.io/platform: "true"
spec:
  path: voltron-dev
  stateStoreRef:
    name: github-store
---
apiVersion: platform.kratix.io/v1alpha1
kind: Destination
metadata:
  name: prod-cluster
  labels:
    environment: prod
    infraforge.io/platform: "true"
spec:
  path: voltron-prod
  stateStoreRef:
    name: github-store
```

## 🚨 Current Limitations

1. **No Operator Management**:
   - Operators need manual installation
   - No version control for operators
   - No operator lifecycle management

2. **Single Bootstrap App**:
   - All environments in one app
   - No environment-specific sync policies
   - Hard to manage at scale

3. **No Resource Segregation**:
   - All resources in same cluster
   - No node selectors
   - No resource quotas

## 📋 Immediate Actions Needed

### 1. Test Multi-Environment:
```bash
# Create test environment claim
cat <<EOF | kubectl apply -f -
apiVersion: platform.infraforge.io/v1
kind: InfraForge
metadata:
  name: demo-nginx-test
spec:
  tenant: demo
  environment: test
  services:
  - name: web
    type: nginx
    profile: standard
EOF
```

### 2. Add Environment-Specific Configs:
```go
// generator should support env-specific overrides
if environment == "prod" {
    // Add PDB, HPA, etc.
}
```

### 3. Operator Bootstrap Strategy:
```yaml
# Option 1: Pre-install all operators
# Option 2: Install on-demand via separate pipeline
# Option 3: Bundle with platform
```

## 🎯 Next Steps

1. **Test multi-env deployment** (now)
2. **Add resource quotas per namespace** (critical)
3. **Implement operator lifecycle** (next week)
4. **Multi-cluster support** (future)

## Questions to Resolve:

1. **Operator Installation**:
   - When: Bootstrap time or on-demand?
   - Where: Platform namespace or per-tenant?
   - How: Helm charts or raw manifests?

2. **Environment Promotion**:
   - Manual PR process?
   - Automated promotion?
   - Approval gates?

3. **Secret Management**:
   - Per environment secrets?
   - Cross-environment sharing?
   - Rotation strategy?