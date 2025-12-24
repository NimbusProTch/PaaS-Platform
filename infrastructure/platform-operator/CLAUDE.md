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
- ✅ Renamed PlatformClaim to PlatformApplicationClaim for clarity
- ✅ Added missing fields to ApplicationSpec (serviceName, version)
- ✅ Added missing fields to ComponentSpec (storage, replicas, resources)

### Latest Changes (Session 3 - CRD Refactoring)
- ✅ Renamed PlatformClaim → PlatformApplicationClaim
- ✅ Updated all references in controller and main.go
- ✅ Updated deployment claims to use correct kinds
- ✅ Added clusterType field to all claims
- ✅ Generated new CRDs with updated types

## Custom Resource Definitions (CRDs)

### 1. ApplicationClaim
**Purpose:** Deploy business applications (microservices)

**File:** `deployments/dev/apps-claim.yaml`

**Kind:** `ApplicationClaim`

**Spec Fields:**
- `environment`: dev, qa, sandbox, staging, prod
- `clusterType`: nonprod, prod
- `owner`: Team ownership information
- `applications`: List of ApplicationSpec

**ApplicationSpec Fields:**
- `name`: Application name
- `serviceName`: Kubernetes service name (optional)
- `version`: Application version
- `chart`: Helm chart configuration (optional if using image directly)
- `image`: Container image config
- `replicas`: Number of replicas
- `resources`: CPU/memory requirements
- `ports`: Exposed ports
- `healthCheck`: Health check configuration
- `env`: Environment variables
- `autoscaling`: HPA configuration
- `ingress`: Ingress configuration

**Controller:** `ApplicationClaimGitOpsReconciler`

**Generated Files:**
- `appsets/{clusterType}/apps/{env}-appset.yaml`
- `environments/{clusterType}/{env}/applications/{app-name}/values.yaml`
- `environments/{clusterType}/{env}/applications/{app-name}/config.yaml`

### 2. PlatformApplicationClaim
**Purpose:** Deploy platform infrastructure (PostgreSQL, Redis, RabbitMQ, etc.)

**File:** `deployments/dev/platform-infrastructure-claim.yaml`

**Kind:** `PlatformApplicationClaim`

**Spec Fields:**
- `environment`: dev, qa, sandbox, staging, prod
- `clusterType`: nonprod, prod
- `owner`: Team ownership information
- `services`: List of PlatformServiceSpec

**PlatformServiceSpec Fields:**
- `type`: Service type (postgresql, redis, rabbitmq, elasticsearch, mongodb, mysql, kafka)
- `name`: Instance name
- `version`: Service version
- `chart`: Helm chart configuration
- `values`: Custom Helm values
- `size`: small, medium, large
- `highAvailability`: Enable HA
- `backup`: Backup configuration
- `monitoring`: Enable monitoring

**Controller:** `PlatformApplicationClaimReconciler`

**Generated Files:**
- `appsets/{clusterType}/platform/{env}-platform-appset.yaml`
- `environments/{clusterType}/{env}/platform/{service-name}/values.yaml`

### 3. BootstrapClaim
**Purpose:** Initialize GitOps repository structure

**Kind:** `BootstrapClaim`

**Spec Fields:**
- `organization`: Gitea organization name
- `repositories`: Repositories to create (charts, voltran)
- `gitops`: GitOps configuration (clusterType, environments, branch)

**Controller:** `BootstrapReconciler`

**Generated Structure:**
```
voltran/
  root-apps/
    {clusterType}/
      {clusterType}-apps-rootapp.yaml
      {clusterType}-platform-rootapp.yaml
  appsets/
    {clusterType}/
      apps/.gitkeep
      platform/.gitkeep
  environments/
    {clusterType}/
      {env}/
        applications/.gitkeep
        platform/.gitkeep
```

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

3. **PlatformApplicationClaim Controller** (`platformapplicationclaim_controller.go`):
   - Renamed from PlatformClaim for clarity
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
- `internal/controller/bootstrap_controller.go` - Uses internal URLs, creates 2 root apps
- `internal/controller/applicationclaim_gitops_controller.go` - Git Directory Generator + internal URLs
- `internal/controller/platformapplicationclaim_controller.go` - Renamed from platformclaim_controller.go
- `internal/metrics/collector.go` - Fixed app.Version → app.Image.Tag

### API Types
- `api/v1/applicationclaim_types.go` - Added serviceName, version, updated ComponentSpec
- `api/v1/platformapplicationclaim_types.go` - Renamed from platformclaim_types.go

### Deployment Configuration
- `cmd/manager/main.go` - Updated to use PlatformApplicationClaimReconciler
- `config/manager/deployment.yaml` - Added args, reads token from Secret
- `config/manager/kustomization.yaml` - Added namespace field

### Deployment Claims
- `deployments/dev/apps-claim.yaml` - Added clusterType field
- `deployments/dev/platform-infrastructure-claim.yaml` - Changed to PlatformApplicationClaim kind, added clusterType

## Testing Session Summary

### Test Commands
```bash
# 1. Generate CRDs
make manifests

# 2. Install CRDs
make install

# 3. Run operator locally
export GITEA_TOKEN=<your-token>
go run cmd/manager/main.go \
  --gitea-url=http://localhost:30300 \
  --gitea-username=gitea_admin \
  --charts-path=./charts

# 4. Apply BootstrapClaim
kubectl apply -f config/samples/bootstrap-claim.yaml

# 5. Check bootstrap status
kubectl get bootstrapclaim bootstrap-platform -o yaml

# 6. Apply ApplicationClaim
kubectl apply -f deployments/dev/apps-claim.yaml

# 7. Apply PlatformApplicationClaim
kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml

# 8. Check statuses
kubectl get applicationclaim
kubectl get platformapplicationclaim
```

## Architecture Summary

### Repository Structure
```
platform/charts/           # Helm charts for apps and platform services
  microservice/           # Generic microservice chart
  postgresql/             # PostgreSQL chart
  redis/                  # Redis chart

platform/voltran/         # GitOps configuration
  appsets/               # ApplicationSet definitions
    {clusterType}/apps/{env}-appset.yaml      # App ApplicationSet per environment
    {clusterType}/platform/{env}-platform-appset.yaml  # Platform ApplicationSet per environment
  environments/          # Environment-specific values
    {clusterType}/
      {env}/
        applications/
          {app}/
            values.yaml      # Helm values
            config.yaml      # Chart metadata
        platform/
          {service}/
            values.yaml      # Helm values
  root-apps/             # ArgoCD root apps
    {clusterType}/
      {clusterType}-apps-rootapp.yaml
      {clusterType}-platform-rootapp.yaml
```

### Controllers
- **BootstrapReconciler** - Creates Gitea org, repos, initial structure
- **ApplicationClaimGitOpsReconciler** - Generates ApplicationSet + values for apps
- **PlatformApplicationClaimReconciler** - Generates ApplicationSet + values for platform services

## Next Steps

1. Test Bootstrap flow with actual Gitea instance
2. Test ApplicationClaim with sample apps
3. Test PlatformApplicationClaim with PostgreSQL/Redis
4. Verify ArgoCD can sync from generated structure
5. Test multi-environment deployment (dev, qa, prod)
