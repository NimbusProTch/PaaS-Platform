# 🔴 URGENT FIX REQUIRED - GitHub Token

## Problem
The GitHub token `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` is **INVALID/EXPIRED**.

ArgoCD cannot pull Helm charts from GitHub Packages (ghcr.io) because authentication fails.

## Solution Options

### Option 1: Create New GitHub PAT (Recommended)
1. Go to: https://github.com/settings/tokens
2. Generate new token (classic)
3. Required permissions:
   - `read:packages` ✅
   - `write:packages` ✅ (for pushing)
4. Update the token in:
   - `deployments/argocd-ghcr-secret.yaml`
   - `Makefile` (GITHUB_TOKEN variable)
   - ArgoCD secret

### Option 2: Make Charts Public
```bash
# Run this script after setting valid GITHUB_TOKEN
chmod +x scripts/make-charts-public.sh
./scripts/make-charts-public.sh
```

### Option 3: Use Alternative Registry
- Push charts to Docker Hub (public)
- Use ChartMuseum (local HTTP)
- Store charts in Gitea (git-based)

## Current Status
- ✅ Platform Operator v1.1.0 working
- ✅ GitOps structure created
- ✅ ApplicationSets generated
- ❌ ArgoCD cannot authenticate to ghcr.io
- ❌ Applications stuck in "Unknown" status

## Quick Test
```bash
# Test if token works
echo YOUR_NEW_TOKEN | helm registry login ghcr.io -u nimbusprotch --password-stdin

# Pull a chart
helm pull oci://ghcr.io/nimbusprotch/microservice --version 1.0.0
```

## Update Token in System
```bash
# 1. Update ArgoCD secret
kubectl delete secret oci-ghcr-creds -n argocd
kubectl create secret generic oci-ghcr-creds \
  --from-literal=url=oci://ghcr.io/nimbusprotch \
  --from-literal=type=helm \
  --from-literal=enableOCI=true \
  --from-literal=username=nimbusprotch \
  --from-literal=password=YOUR_NEW_TOKEN \
  -n argocd

kubectl label secret oci-ghcr-creds \
  argocd.argoproj.io/secret-type=repository \
  -n argocd

# 2. Restart ArgoCD
kubectl rollout restart deployment argocd-repo-server -n argocd

# 3. Refresh applications
kubectl get applications -n argocd -o name | xargs -I {} kubectl patch {} -n argocd --type json -p '[{"op": "remove", "path": "/status"}]'
```

## Contact
Please create a new GitHub PAT and update the system configuration.