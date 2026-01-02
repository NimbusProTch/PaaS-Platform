# Quick Reference: Git-Only Architecture

**TL;DR**: ChartMuseum removed. Everything now in Gitea.

---

## 🚀 Quick Start

### 1. Push Charts (One-time Setup)

```bash
export GITEA_TOKEN=$(kubectl get secret -n gitea gitea-admin-secret -o jsonpath='{.data.password}' | base64 -d)
./scripts/push-charts-to-gitea.sh
```

**Result**: Charts available at `http://gitea.com/infraforge/charts`

### 2. Deploy Application

```bash
kubectl apply -f deployments/dev/apps-claim.yaml
```

**Result**: ApplicationSet + Applications created automatically

### 3. Verify

```bash
# Check ApplicationSet
kubectl get applicationset -n argocd

# Check Applications
kubectl get application -n argocd

# Check Pods
kubectl get pods -n dev
```

---

## 📂 Repository Layout

### Gitea: infraforge/charts
```
charts/
├── microservice/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
├── postgresql/
├── redis/
└── ...
```

### Gitea: infraforge/voltran
```
voltran/
├── appsets/
│   └── nonprod/apps/dev-appset.yaml
└── environments/
    └── nonprod/dev/applications/
        └── user-service/
            ├── values.yaml
            └── config.json
```

---

## 🔧 ApplicationSet Structure

### Old (ChartMuseum)
```yaml
source:
  repoURL: http://chartmuseum.chartmuseum.svc:8080
  chart: microservice
  targetRevision: 1.0.0
  helm:
    values: "{{values}}"  # Doesn't work!
```

### New (Git Multi-Source)
```yaml
sources:
  - repoURL: http://gitea.com/infraforge/charts
    path: microservice
    targetRevision: main
    helm:
      valueFiles:
        - $values/environments/nonprod/dev/applications/user-service/values.yaml

  - repoURL: http://gitea.com/infraforge/voltran
    targetRevision: main
    ref: values
```

---

## 🐛 Troubleshooting

### ApplicationSet not generating Applications

```bash
# Check generator
kubectl get applicationset dev-apps -n argocd -o jsonpath='{.spec.generators}' | jq

# Should use 'git.files' not 'list'
```

### Values not applied

```bash
# Check sources
kubectl get application user-service-dev -n argocd -o jsonpath='{.spec.sources}' | jq

# Should have 2 sources with ref: "values"
```

### Chart not found

```bash
# Re-push charts
./scripts/push-charts-to-gitea.sh

# Verify
curl http://gitea.com/api/v1/repos/infraforge/charts/contents
```

---

## ✅ Checklist

### Setup
- [ ] Gitea running
- [ ] Charts pushed to Gitea
- [ ] Operator updated (if running)

### Deployment
- [ ] Apply ApplicationClaim
- [ ] ApplicationSet created (check with kubectl)
- [ ] Applications generated (should appear automatically)
- [ ] Pods running

### Verification
- [ ] ApplicationSet uses Git generators
- [ ] Applications use multi-source
- [ ] Values applied correctly
- [ ] No ChartMuseum references

---

## 📋 Common Commands

```bash
# Get Gitea token
export GITEA_TOKEN=$(kubectl get secret -n gitea gitea-admin-secret -o jsonpath='{.data.password}' | base64 -d)

# Push charts
./scripts/push-charts-to-gitea.sh

# List ApplicationSets
kubectl get applicationset -n argocd

# List Applications
kubectl get application -n argocd

# Describe ApplicationSet
kubectl describe applicationset dev-apps -n argocd

# Get ApplicationSet YAML
kubectl get applicationset dev-apps -n argocd -o yaml

# Check Application sources
kubectl get application user-service-dev -n argocd -o jsonpath='{.spec.sources}' | jq

# Restart operator (if needed)
kubectl rollout restart deployment -n platform-operator-system platform-operator-controller-manager

# View operator logs
kubectl logs -n platform-operator-system -l control-plane=controller-manager -f
```

---

## 🔄 Update Workflows

### Update Chart

```bash
# 1. Modify chart in /charts/microservice
# 2. Push to Gitea
./scripts/push-charts-to-gitea.sh

# 3. ArgoCD will auto-sync (if enabled)
```

### Update Values

```bash
# 1. Modify claim or edit in Gitea directly
# 2. If using claim:
kubectl apply -f deployments/dev/apps-claim.yaml

# 3. ArgoCD will auto-sync
```

### Add New Service

```bash
# 1. Add to ApplicationClaim
spec:
  applications:
    - name: new-service
      enabled: true
      ...

# 2. Apply
kubectl apply -f deployments/dev/apps-claim.yaml

# 3. Operator creates new Application automatically
```

---

## 📚 Full Documentation

- **Architecture**: `/docs/ARCHITECTURE-GIT-ONLY.md`
- **Migration**: `/docs/MIGRATION-SUMMARY.md`
- **Scripts**: `/scripts/README.md`

---

**Status**: ✅ Production Ready
**Version**: 4.0.0
**Last Updated**: 2025-12-29
