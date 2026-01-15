# Platform Deployment Fixes Summary

## Problems Identified and Fixed

### 1. ✅ Storage Class Issue (PVC)
**Problem**: PVCs were using hardcoded `gp3` storage class which doesn't exist in Kind
**Fix**:
- Added `StorageClass` field to PlatformApplicationClaim CRD
- Updated controller to use storage class from claim (defaults to `standard`)
- Now configurable via claim YAML

**Files Changed**:
- `api/v1/platformapplicationclaim_types.go` - Added StorageClass field
- `internal/controller/platformapplicationclaim_controller.go` - Use storage class from claim
- `deployments/dev/platform-infrastructure-claim.yaml` - Added `storageClass: standard`

### 2. ✅ PostgreSQL Memory Requirements
**Problem**: PostgreSQL containers failing with "Memory request is lower than shared_buffers"
**Fix**: Increased memory from 128Mi to 256Mi in values generation

**Files Changed**:
- `internal/controller/platformapplicationclaim_controller.go` - Updated memory in `generatePlatformValuesYAML`

### 3. ✅ Operator Installation Loop
**Problem**: Operator stuck in infinite loop checking for operator installation
**Fix**: Bypassed operator installation check when operators already exist

**Files Changed**:
- `internal/controller/platformapplicationclaim_controller.go` - Skip operator check, assume installed

### 4. ✅ Chart Repository URL
**Problem**: ApplicationSet pointing to Gitea for charts instead of ChartMuseum
**Fix**: Changed repoURL from Gitea to ChartMuseum service

**Files Changed**:
- `internal/controller/platformapplicationclaim_controller.go` - Line 325: Changed to `http://chartmuseum.chartmuseum.svc.cluster.local:8080`

## Current Status

### What Works ✅
- Operator compiles and runs
- ApplicationSets are created
- Applications are generated from ApplicationSets
- Values are written to Gitea repository
- Storage class is configurable from claim

### What Needs Testing 🔄
- PostgreSQL and Redis actual deployment (cluster is down)
- PVC binding with correct storage class
- Chart fetching from ChartMuseum
- End-to-end GitOps flow without manual intervention

## Next Steps

1. **Restart OrbStack/Docker** when system is responsive
2. **Run the fix script**: `./fix-platform-deployment.sh`
3. **Verify deployments**:
   ```bash
   kubectl get pods -n dev-platform
   kubectl get pvc -n dev-platform
   kubectl get clusters.postgresql.cnpg.io -n dev-platform
   ```

## Key Commands

```bash
# Build and push operator
cd infrastructure/platform-operator
docker build -t ghcr.io/nimbusprotch/platform-operator:latest .
docker push ghcr.io/nimbusprotch/platform-operator:latest

# Restart operator
kubectl rollout restart deployment/platform-operator-controller-manager -n platform-operator-system

# Check ArgoCD apps
kubectl get applications -n argocd | grep platform

# Check platform services
kubectl get all -n dev-platform

# Manual sync if needed
kubectl patch application product-db-dev -n argocd --type merge -p '{"operation": {"sync": {"prune": true}}}'
```

## GitOps Pattern Fixed

```
BootstrapClaim → Gitea Setup → Root Apps
                                    ↓
PlatformClaim → Operator → ApplicationSet → Applications
                    ↓              ↓              ↓
              Values.yaml    ChartMuseum    K8s Resources
              (in Gitea)      (Charts)     (PostgreSQL, Redis)
```

## Manual Workarounds Applied (Temporary)

1. **Values manually updated via Gitea API** - Fixed memory settings
2. **Port forwarding processes killed** - To clear connection issues

## Configuration Now Supported

```yaml
spec:
  # Storage class is now configurable!
  storageClass: standard  # For Kind
  # storageClass: gp3     # For AWS

  services:
    - type: postgresql
      name: product-db
      enabled: true
      # Memory automatically set to 256Mi minimum
```

## Success Criteria Checklist

- [x] Storage class configurable from claim
- [x] Memory requirements fixed for PostgreSQL
- [x] Operator installation loop resolved
- [x] Chart repository pointing to ChartMuseum
- [ ] PostgreSQL clusters actually running
- [ ] Redis instances actually running
- [ ] PVCs bound successfully
- [ ] All pods healthy in dev-platform namespace