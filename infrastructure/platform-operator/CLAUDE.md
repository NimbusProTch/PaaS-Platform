# Platform Operator Development Session Notes

## Current Status

### Completed
- ✅ Simplified Gitea deployment (SQLite, no HA cluster)
- ✅ Fixed internal communication (cluster-internal URLs)
- ✅ Added automated token generation via init job
- ✅ Fixed metrics collector compilation error
- ✅ Updated all controllers to use Git Directory Generator pattern
- ✅ Created ConstructCloneURL() method for internal URLs
- ✅ Fixed GitOps directory structure with cluster-type hierarchy
- ✅ Fixed all controllers to generate correct file paths and naming

### Latest Changes (Session 2)
- ✅ Fixed Bootstrap controller to create proper directory hierarchy
- ✅ Fixed ApplicationClaim controller file paths and naming
- ✅ Fixed PlatformClaim controller file paths and naming
- ✅ All appsets now generate as `{env}-appset.yaml` (not `-applications.yaml`)
- ✅ All paths now include cluster type separation (nonprod/prod)
- ✅ Fixed Bootstrap to create 2 separate root apps (apps & platform)
- ✅ Fixed PlatformClaim appset naming to `{env}-platform-appset.yaml`

## Fixed GitOps Structure

The corrected directory structure in voltran repository:

```
voltran/
  root-apps/
    nonprod/
      nonprod-apps-rootapp.yaml      ← Points to appsets/nonprod/apps
      nonprod-platform-rootapp.yaml  ← Points to appsets/nonprod/platform
    prod/
      prod-apps-rootapp.yaml         ← Points to appsets/prod/apps
      prod-platform-rootapp.yaml     ← Points to appsets/prod/platform

  appsets/
    nonprod/
      apps/
        dev-appset.yaml      ← ApplicationSet for dev applications
        qa-appset.yaml       ← ApplicationSet for qa applications
        sandbox-appset.yaml  ← ApplicationSet for sandbox applications
      platform/
        dev-platform-appset.yaml      ← ApplicationSet for dev platform services
        qa-platform-appset.yaml       ← ApplicationSet for qa platform services
    prod/
      apps/
        prod-appset.yaml     ← ApplicationSet for prod applications
        stage-appset.yaml    ← ApplicationSet for stage applications
      platform/
        prod-platform-appset.yaml     ← ApplicationSet for prod platform services

  environments/
    nonprod/
      dev/
        applications/        ← Business app values
          api-gateway/
            values.yaml
            config.yaml
          product-service/
            values.yaml
            config.yaml
        platform/           ← Platform service values
          postgresql/
            values.yaml
          redis/
            values.yaml
      qa/
        applications/
        platform/
    prod/
      prod/
        applications/
        platform/
      stage/
        applications/
        platform/
```

### Key Changes Made

1. **Bootstrap Controller** (`bootstrap_controller.go`):
   - Created 2 separate root apps for each cluster type:
     - `root-apps/{clusterType}/{clusterType}-apps-rootapp.yaml` → points to `appsets/{clusterType}/apps`
     - `root-apps/{clusterType}/{clusterType}-platform-rootapp.yaml` → points to `appsets/{clusterType}/platform`
   - Created hierarchy: `appsets/{clusterType}/{apps|platform}/`
   - Created hierarchy: `environments/{clusterType}/{env}/{applications|platform}/`

2. **ApplicationClaim Controller** (`applicationclaim_gitops_controller.go`):
   - Appset file: `appsets/{env}-applications.yaml` → `appsets/{clusterType}/apps/{env}-appset.yaml`
   - Values path: `environments/{env}/{app}/` → `environments/{clusterType}/{env}/applications/{app}/`
   - Git generator path updated to match new structure

3. **PlatformClaim Controller** (`platformclaim_controller.go`):
   - Appset file: `appsets/{env}-platform.yaml` → `appsets/{clusterType}/platform/{env}-platform-appset.yaml`
   - Values path: `environments/{env}/{service}-values.yaml` → `environments/{clusterType}/{env}/platform/{service}/values.yaml`
   - ValueFiles path in ApplicationSet updated to match new structure
   - Fixed repoURL from `platform-charts` to `charts`

## Files Modified in This Session

### Infrastructure Scripts
- `scripts/deploy-gitea.sh` - Simplified to SQLite, added ROOT_URL/DOMAIN for internal URLs
- `scripts/init-gitea-token.yaml` - NEW: Automated token generation Kubernetes Job

### Operator Code
- `pkg/gitea/client.go` - Added ConstructCloneURL() method
- `internal/controller/bootstrap_controller.go` - Uses internal URLs (line 115)
- `internal/controller/applicationclaim_gitops_controller.go` - Git Directory Generator + internal URLs
- `internal/controller/platformclaim_controller.go` - Uses internal URLs (line 85)
- `internal/metrics/collector.go` - Fixed app.Version → app.Image.Tag (line 239)

### Deployment Configuration
- `config/manager/deployment.yaml` - Added args, reads token from Secret
- `config/manager/kustomization.yaml` - Added namespace field

## Testing Session Summary

### Test Commands
```bash
# 1. Tear down and recreate Kind cluster
make dev-down
make dev-up

# 2. Port-forward to Gitea (for local operator)
kubectl port-forward -n gitea svc/gitea-http 30300:3000 &

# 3. Install CRDs
make install

# 4. Run operator locally
export GITEA_TOKEN=b3bb42500ac403d5c162a71e4fb442ceb4c7b25a
go run cmd/manager/main.go --gitea-url=http://localhost:30300 --gitea-username=gitea_admin

# 5. Apply BootstrapClaim
kubectl apply -f config/samples/bootstrap-claim.yaml

# 6. Check status
kubectl get bootstrapclaim bootstrap-platform -o yaml
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
  Message: "Failed to load charts: failed to walk charts directory: lstat /charts: no such file or directory"
```

## Current Bootstrap Flow

### What's Working (90%)
1. ✅ Organization "platform" created in Gitea
2. ✅ Repositories created:
   - platform/charts (for Helm charts)
   - platform/voltran (for GitOps structure)
3. ⏸️ Charts upload - **BLOCKED** by missing ChartsPath

### What's Failing
- Charts upload failing because ChartsPath parameter not set
- Operator trying to read from `/charts` but path not provided

## Next Actions

### 1. Add ChartsPath Parameter to Operator
```go
// cmd/manager/main.go
var chartsPath string
flag.StringVar(&chartsPath, "charts-path", "", "Path to charts directory")

// Pass to Bootstrap Controller
err = (&controller.BootstrapReconciler{
    Client:      mgr.GetClient(),
    Scheme:      mgr.GetScheme(),
    GiteaClient: giteaClient,
    ChartsPath:  chartsPath,
}).SetupWithManager(mgr)
```

### 2. Restart Operator with Charts Path
```bash
export GITEA_TOKEN=b3bb42500ac403d5c162a71e4fb442ceb4c7b25a
go run cmd/manager/main.go \
  --gitea-url=http://localhost:30300 \
  --gitea-username=gitea_admin \
  --charts-path=./charts
```

### 3. Verify Bootstrap Completion
```bash
kubectl get bootstrapclaim bootstrap-platform -o yaml
kubectl logs -n gitea deployment/gitea -c gitea
```

### 4. Deploy ArgoCD (After Bootstrap Success)
```bash
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

### 5. Test ApplicationClaim (After Bootstrap Success)
```bash
kubectl apply -f ecommerce-claim.yaml
kubectl get applicationclaim -n default ecommerce-platform -o yaml
```

## Session Notes

### Key Learnings
1. **Git Directory Generator** pattern requires directory structure:
   - `environments/{env}/{app}/values.yaml`
   - `environments/{env}/{app}/config.yaml`

2. **Internal URLs** are critical for cluster communication:
   - Gitea ROOT_URL: `http://gitea-http.gitea.svc.cluster.local:3000`
   - Clone URLs constructed by operator, not from API

3. **Local Testing** requires port-forward:
   - Operator can't resolve cluster DNS when running outside cluster
   - Use `kubectl port-forward` + `localhost` URLs

### Pending Items
- [ ] Fix ChartsPath parameter issue
- [ ] Complete bootstrap flow
- [ ] Debug init job token generation (currently using manual token)
- [ ] Deploy ArgoCD
- [ ] Test ApplicationClaim
- [ ] Test PlatformClaim

## Architecture Summary

### Repository Structure
```
platform/charts/           # Helm charts for apps and platform services
  microservice/           # Generic microservice chart
  postgresql/             # PostgreSQL chart
  redis/                  # Redis chart

platform/voltran/         # GitOps configuration
  appsets/               # ApplicationSet definitions
    {env}-apps.yaml      # App ApplicationSet per environment
    {env}-platform.yaml  # Platform ApplicationSet per environment
  environments/          # Environment-specific values
    dev/
      {app}/
        values.yaml      # Helm values
        config.yaml      # Chart metadata
    qa/
      ...
  root-apps/             # ArgoCD root apps
    {cluster}-root.yaml
```

### Controllers
- **BootstrapReconciler** - Creates Gitea org, repos, initial structure
- **ApplicationClaimGitOpsReconciler** - Generates ApplicationSet + values for apps
- **PlatformClaimReconciler** - Generates ApplicationSet + values for platform services
