# InfraForge Platform - Architecture Documentation

**Last Updated**: 2025-12-24 22:51 UTC+3
**Status**: 🔄 Local Testing Phase
**Phase**: Bootstrap Controller Implementation & Testing

---

## 🎯 Current Status

### ✅ Completed Today (2025-12-24)
1. **Gitea Deployment Simplified**
   - Removed PostgreSQL HA cluster (3 pods) → SQLite
   - Removed Valkey cluster (3 pods) → Memory cache
   - Single pod deployment for local testing
   - ROOT_URL set to internal: `http://gitea-http.gitea.svc.cluster.local:3000`

2. **Token Automation**
   - Created `init-gitea-token.yaml` (Kubernetes Job)
   - Automatic token generation using Gitea CLI
   - Token stored in Secret: `gitea-token` (platform-operator-system namespace)
   - Deployment reads from Secret instead of hardcoded value

3. **Internal Communication Fixed**
   - Added `ConstructCloneURL()` method to Gitea client
   - All controllers use internal URL construction
   - Bootstrap, ApplicationClaim, PlatformClaim updated
   - No external DNS lookups

4. **Git Directory Generator Implementation**
   - ApplicationClaim uses Git Directory Generator
   - Each app has: `environments/{env}/{app}/values.yaml` + `config.yaml`
   - ArgoCD discovers apps automatically by scanning directories

5. **Operator Local Testing**
   - CRDs installed successfully
   - Operator running locally
   - Port-forward to Gitea: `localhost:30300`
   - BootstrapClaim applied

### ⚠️ Current Issue
**Problem**: Bootstrap failing at charts upload phase
```
Message: Failed to load charts: failed to walk charts directory: lstat /charts: no such file or directory
```

**Root Cause**: `ChartsPath` parameter not set when starting operator

**Status**:
- ✅ Gitea organization created: `platform`
- ✅ Repositories created: `charts`, `voltran`
- ❌ Charts upload failed - missing charts path parameter

### 🔧 Next Step
Set `--charts-path` parameter and restart operator:
```bash
export GITEA_TOKEN=b3bb42500ac403d5c162a71e4fb442ceb4c7b25a
kubectl port-forward -n gitea svc/gitea-http 30300:3000 &
go run cmd/manager/main.go \
  --gitea-url=http://localhost:30300 \
  --gitea-username=gitea_admin \
  --charts-path=/Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform/charts
```

---

## 🏗️ System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         LOCAL TEST ENVIRONMENT                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│  │  Kind       │  │   Gitea     │  │   ArgoCD    │                 │
│  │  Cluster    │  │  (SQLite)   │  │  (planned)  │                 │
│  └─────────────┘  └─────────────┘  └─────────────┘                 │
│                                                                      │
│  ┌──────────────────────────────────────────────────────┐           │
│  │  Platform Operator (Running Locally)                 │           │
│  │  - Port-forward to Gitea: localhost:30300            │           │
│  │  - Charts path: /Users/.../PaaS-Platform/charts      │           │
│  └──────────────────────────────────────────────────────┘           │
│                                                                      │
│  kubectl apply -f bootstrap-claim.yaml  ← Testing Now               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 📂 Files Modified in This Session

### 1. Gitea Deployment
**File**: `scripts/deploy-gitea.sh`
- Added `ROOT_URL` and `DOMAIN` for internal cluster URLs
- Simplified to SQLite + memory cache
- Added automatic init job execution
- Token stored in Secret

**File**: `scripts/init-gitea-token.yaml` (NEW)
- Kubernetes Job for automated token generation
- RBAC permissions included
- Idempotent (won't recreate if exists)

### 2. Gitea Client
**File**: `infrastructure/platform-operator/pkg/gitea/client.go`
- Added `ConstructCloneURL()` method
- Builds cluster-internal URLs instead of using API response

### 3. Controllers
**File**: `internal/controller/bootstrap_controller.go` (Line 115)
- Uses `ConstructCloneURL()` for internal URLs

**File**: `internal/controller/applicationclaim_gitops_controller.go` (Line 91)
- Uses Git Directory Generator
- Uses `ConstructCloneURL()`

**File**: `internal/controller/platformclaim_controller.go` (Line 85)
- Uses `ConstructCloneURL()`

**File**: `internal/metrics/collector.go` (Line 239)
- Fixed: `app.Version` → `app.Image.Tag`

### 4. Deployment Config
**File**: `config/manager/deployment.yaml`
- Added `--gitea-url` and `--gitea-username` args
- Token read from Secret instead of hardcoded

**File**: `config/manager/kustomization.yaml`
- Added `namespace: platform-operator-system`

---

## 🔬 Testing Session Summary

### Environment Setup
```bash
# 1. Start clean Kind cluster
make dev-down && make dev-up

# 2. Deploy Gitea (simplified)
scripts/deploy-gitea.sh
# Result: Single pod deployment with SQLite

# 3. Manual token generation (temporary until job is fixed)
kubectl exec -n gitea <pod> -c gitea -- \
  gitea admin user generate-access-token \
  --username gitea_admin \
  --token-name platform-operator-manual \
  --scopes write:organization,write:repository,write:user

# 4. Create secret
kubectl create secret generic gitea-token \
  --namespace platform-operator-system \
  --from-literal=token=<TOKEN>

# 5. Install CRDs
make install

# 6. Port-forward Gitea
kubectl port-forward -n gitea svc/gitea-http 30300:3000 &

# 7. Run operator locally
export GITEA_TOKEN=<TOKEN>
go run cmd/manager/main.go \
  --gitea-url=http://localhost:30300 \
  --gitea-username=gitea_admin

# 8. Apply BootstrapClaim
kubectl apply -f bootstrap-claim.yaml
```

### Current Bootstrap Status
```yaml
Status:
  Phase: Failed
  Ready: false
  Repositories Created: true
  Repository URLs:
    Charts: http://localhost:30300/platform/charts.git
    Voltran: http://localhost:30300/platform/voltran.git
  Charts Uploaded: false
  Message: "Failed to load charts: no such file or directory: /charts"
```

---

## 📋 Gitea Repository Structure (Actual)

```
Gitea: http://localhost:30300
Organization: platform

├── 📦 charts/                         ✅ Created (empty)
│   └── (waiting for charts to be pushed)
│
└── 📦 voltran/                        ✅ Created (empty)
    └── (waiting for GitOps structure)
```

---

## 🚀 Next Actions

### Immediate (Fix Bootstrap)
1. **Add Charts Path Parameter**
   ```go
   // cmd/manager/main.go
   flag.StringVar(&chartsPath, "charts-path", "", "Path to embedded charts directory")
   ```

2. **Pass ChartsPath to Bootstrap Controller**
   ```go
   // Pass when setting up controller
   ChartsPath: chartsPath,
   ```

3. **Restart Operator with Path**
   ```bash
   go run cmd/manager/main.go \
     --gitea-url=http://localhost:30300 \
     --gitea-username=gitea_admin \
     --charts-path=/Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform/charts
   ```

4. **Verify Bootstrap Completes**
   ```bash
   kubectl get bootstrapclaim -A
   # Expected: Phase: Ready, Ready: true
   ```

### After Bootstrap Success
1. Test ApplicationClaim with ecommerce-platform
2. Test PlatformClaim with PostgreSQL
3. Deploy ArgoCD
4. Verify end-to-end GitOps flow

---

## 📞 Session Notes

### Key Learnings
1. **Local Testing**: Port-forward works better than NodePort for local operator
2. **Init Job Issue**: Token generation works manually, job needs debugging
3. **Git Directory Generator**: Correctly implemented, waiting for charts to test
4. **Internal URLs**: All fixed, using `ConstructCloneURL()`

### Pending Items
1. Fix init-gitea-token.yaml job (token generation failing in container)
2. Add charts-path parameter to operator
3. Complete bootstrap flow end-to-end
4. Deploy ArgoCD for full GitOps testing

---

**Last Updated**: 2025-12-24 22:51 UTC+3
**Next Session**: Fix ChartsPath parameter and complete bootstrap
