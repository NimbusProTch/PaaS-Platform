# Migration Summary: ChartMuseum to Git-Only Architecture

**Date**: 2025-12-29
**Status**: ✅ Code Complete, Ready for Testing
**Impact**: Breaking Change - ApplicationSets regenerated

---

## 🎯 What Changed

### Architecture Simplification

**Before**:
```
Charts (ChartMuseum OCI) + Values (Gitea Git) = Hybrid Architecture
- ArgoCD couldn't use valueFiles with OCI charts
- Two separate push workflows
- Complex configuration
```

**After**:
```
Charts (Gitea Git) + Values (Gitea Git) = Git-Only Architecture
- ArgoCD multi-source with valueFiles support
- Single Git workflow
- Simple, clean architecture
```

---

## 📋 Changes Made

### 1. New Script: push-charts-to-gitea.sh

**Location**: `/scripts/push-charts-to-gitea.sh`

**Purpose**: Automates pushing Helm charts to Gitea charts repository

**Features**:
- Creates organization and repository automatically
- Copies all charts from local `/charts` directory
- Commits and pushes to Gitea
- Idempotent (safe to run multiple times)

**Usage**:
```bash
export GITEA_TOKEN=<token>
./scripts/push-charts-to-gitea.sh
```

---

### 2. Controller Updates

#### File: applicationclaim_gitops_controller.go

**Function**: `generateApplication()`

**Changes**:
```go
// OLD - ChartMuseum
source: {
  repoURL: "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
  chart: chartName,
  targetRevision: version,
  helm: {
    values: string(valuesYAML)  // Inline values
  }
}

// NEW - Git Source
source: {
  repoURL: fmt.Sprintf("%s/%s/charts", claim.Spec.GiteaURL, claim.Spec.Organization),
  path: chartName,
  targetRevision: "main",
  helm: {
    values: string(valuesYAML)
  }
}
```

**Function**: `generateApplicationSet()`

**Changes**:
```go
// OLD - List Generator + ChartMuseum
generators: [
  {
    list: {
      elements: [...]  // Inline list
    }
  }
]
template: {
  spec: {
    source: {
      repoURL: "http://chartmuseum...",
      chart: "microservice",
      helm: {
        values: "{{values}}"  // String interpolation doesn't work
      }
    }
  }
}

// NEW - Git Files Generator + Multi-Source
generators: [
  {
    git: {
      repoURL: "gitea.com/org/voltran",
      files: [
        {path: "environments/nonprod/dev/applications/*/config.json"}
      ]
    }
  }
]
template: {
  spec: {
    sources: [
      {
        // Chart from Git
        repoURL: "gitea.com/org/charts",
        path: "{{chart}}",
        helm: {
          valueFiles: ["$values/environments/.../{{name}}/values.yaml"]
        }
      },
      {
        // Values reference
        repoURL: "gitea.com/org/voltran",
        ref: "values"
      }
    ]
  }
}
```

#### File: platformapplicationclaim_controller.go

**Same changes applied to**:
- `generatePlatformApplication()`
- `generatePlatformApplicationSet()`

---

### 3. Documentation Updates

#### New Document: ARCHITECTURE-GIT-ONLY.md

**Location**: `/docs/ARCHITECTURE-GIT-ONLY.md`

**Contents**:
- Complete architecture overview
- Repository structure
- Workflow diagrams
- Implementation details
- Quick start guide
- Troubleshooting
- Comparison table

#### Updated: scripts/README.md

**Added**:
- Script details for `push-charts-to-gitea.sh`
- Environment variables
- Usage examples

---

## 🔄 Migration Steps

### For New Deployments

```bash
# 1. Create infrastructure
make kind-create
make install-operator
make install-gitea
make install-argocd

# 2. Push charts to Gitea
export GITEA_TOKEN=$(kubectl get secret -n gitea gitea-admin-secret -o jsonpath='{.data.password}' | base64 -d)
./scripts/push-charts-to-gitea.sh

# 3. Deploy claims
kubectl apply -f deployments/dev/apps-claim.yaml
kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml

# 4. Verify
kubectl get applicationset -n argocd
kubectl get application -n argocd
```

### For Existing Deployments

```bash
# 1. Push charts to Gitea
export GITEA_TOKEN=<your-token>
./scripts/push-charts-to-gitea.sh

# 2. Update operator (rebuild with new code)
cd infrastructure/platform-operator
make docker-build
make deploy

# 3. Delete old ApplicationSets
kubectl delete applicationset -n argocd -l platform.infraforge.io/environment=dev

# 4. Re-apply claims (will create new ApplicationSets)
kubectl delete -f deployments/dev/apps-claim.yaml
kubectl apply -f deployments/dev/apps-claim.yaml

# 5. Verify new ApplicationSets
kubectl get applicationset -n argocd -o yaml | grep repoURL
# Should show gitea URLs, not chartmuseum
```

---

## ✅ Verification Checklist

### Charts Repository

- [ ] Organization `infraforge` exists in Gitea
- [ ] Repository `charts` exists
- [ ] All charts present: microservice, postgresql, redis, mongodb, rabbitmq, kafka
- [ ] Each chart has Chart.yaml, values.yaml, templates/

```bash
# Verify
curl http://gitea-http.gitea.svc.cluster.local:3000/api/v1/repos/infraforge/charts/contents
```

### ApplicationSets

- [ ] ApplicationSets use Git generators (not List)
- [ ] ApplicationSets use multi-source (sources array)
- [ ] First source points to charts repo
- [ ] Second source points to voltran repo with ref: values
- [ ] valueFiles use $values/ prefix

```bash
# Verify
kubectl get applicationset dev-apps -n argocd -o jsonpath='{.spec.generators[0]}' | jq
kubectl get applicationset dev-apps -n argocd -o jsonpath='{.spec.template.spec.sources}' | jq
```

### Applications

- [ ] Applications generated from ApplicationSets
- [ ] Applications sync successfully
- [ ] Pods deployed correctly
- [ ] Values applied from voltran repo

```bash
# Verify
kubectl get application -n argocd
kubectl get pods -n dev
```

---

## 🐛 Known Issues & Solutions

### Issue 1: ApplicationSet not generating Applications

**Symptoms**: ApplicationSet exists but no Applications created

**Causes**:
- config.json files missing
- Git generator path pattern incorrect
- Gitea not accessible

**Solution**:
```bash
# Check if config.json exists
kubectl exec -n argocd <argocd-pod> -- git clone http://gitea.com/org/voltran /tmp/voltran
kubectl exec -n argocd <argocd-pod> -- ls -la /tmp/voltran/environments/nonprod/dev/applications/*/config.json

# Check ApplicationSet status
kubectl describe applicationset dev-apps -n argocd
```

### Issue 2: Values not being applied

**Symptoms**: Application deploys but uses default values

**Causes**:
- valueFiles path incorrect
- $values/ prefix missing
- Second source missing ref: values

**Solution**:
```bash
# Check Application sources
kubectl get application user-service-dev -n argocd -o jsonpath='{.spec.sources}' | jq

# Should show two sources:
# 1. Chart source with valueFiles: ["$values/..."]
# 2. Values source with ref: "values"
```

### Issue 3: Chart not found

**Symptoms**: Application shows "path not found" error

**Causes**:
- Chart not pushed to Gitea
- Path in ApplicationSet incorrect
- Repository URL wrong

**Solution**:
```bash
# Re-run push script
./scripts/push-charts-to-gitea.sh

# Verify chart exists
curl http://gitea.com/api/v1/repos/infraforge/charts/contents/microservice
```

---

## 📊 Benefits Achieved

### Simplicity
- ✅ Single source of truth (Gitea)
- ✅ No ChartMuseum to maintain
- ✅ Unified Git workflow

### GitOps Native
- ✅ Everything version controlled
- ✅ Full audit trail
- ✅ Easy rollbacks with git

### Multi-Source Power
- ✅ Charts stable in charts repo
- ✅ Values per environment in voltran repo
- ✅ Clean separation of concerns

### Scalability
- ✅ Add environments = add directories
- ✅ Add organizations = new Gitea orgs
- ✅ No infrastructure changes needed

---

## 🚧 Breaking Changes

### ApplicationSets

**Impact**: ApplicationSets will be regenerated with different structure

**Migration**: Delete old ApplicationSets, re-apply claims

**Downtime**: Minimal - Applications will be recreated

### Applications

**Impact**: Applications will have different source configuration

**Migration**: Automatic when ApplicationSets regenerate

**Downtime**: Brief during transition

### Claims

**Impact**: No changes to claim structure

**Migration**: None required (but re-apply to regenerate resources)

---

## 📈 Testing Plan

### Unit Tests
- [ ] Test ApplicationSet generation with Git sources
- [ ] Test multi-source configuration
- [ ] Test valueFiles path construction

### Integration Tests
1. [ ] Deploy to local Kind cluster
2. [ ] Push charts to Gitea
3. [ ] Apply ApplicationClaim
4. [ ] Verify ApplicationSet created
5. [ ] Verify Applications generated
6. [ ] Verify pods deployed with correct values

### End-to-End Tests
1. [ ] Multi-environment setup (dev, qa, staging)
2. [ ] Update values in voltran repo
3. [ ] Verify ArgoCD auto-sync
4. [ ] Update chart in charts repo
5. [ ] Verify new version deployed

---

## 🎯 Next Steps

### Immediate (Required for Testing)
1. ✅ Code changes complete
2. ✅ Documentation complete
3. ⏳ Run push-charts-to-gitea.sh
4. ⏳ Test with live claims
5. ⏳ Verify multi-source ApplicationSets work

### Short-term (Production Readiness)
1. ⏳ Add Makefile target for chart push
2. ⏳ Add validation tests
3. ⏳ Update CI/CD pipelines
4. ⏳ Production deployment guide

### Long-term (Enhancements)
1. ⏳ Chart versioning strategy
2. ⏳ Automated chart updates
3. ⏳ Monitoring for Git repositories
4. ⏳ Backup/restore procedures

---

## 📞 Support

### Documentation
- Architecture: `/docs/ARCHITECTURE-GIT-ONLY.md`
- This summary: `/docs/MIGRATION-SUMMARY.md`
- Script usage: `/scripts/README.md`

### Files Modified
- `/infrastructure/platform-operator/internal/controller/applicationclaim_gitops_controller.go`
- `/infrastructure/platform-operator/internal/controller/platformapplicationclaim_controller.go`
- `/scripts/push-charts-to-gitea.sh` (new)
- `/docs/ARCHITECTURE-GIT-ONLY.md` (new)
- `/scripts/README.md` (updated)

### Repository
https://github.com/NimbusProTch/PaaS-Platform

---

**Version**: 4.0.0
**Date**: 2025-12-29
**Author**: Platform Engineering Team
**Status**: ✅ Ready for Testing
