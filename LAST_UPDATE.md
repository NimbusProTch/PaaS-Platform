# Last Update - 2025-06-29

## 🔄 What Was Done

### 1. Fixed ApplicationSets
- Changed from `directories` generator to `files` generator
- Now scans for specific file patterns:
  - Business apps: `manifests/apps/dev/business-apps/*/values.yaml`
  - Platform apps: `manifests/apps/dev/platform-apps/*/Chart.yaml`
  - Operators: `manifests/operators/dev/*/*.yaml`

### 2. Updated Generator (infraforge-generator)
- **Business Apps**: Now creates only `Chart.yaml` and `values.yaml` for Helm deployments
- **Platform Apps**: Creates `Chart.yaml` and `values.yaml` for Helm deployments
- **Operators**: Creates Custom Resource (CR) YAML files:
  - PostgreSQL: Creates `postgresql-cluster.yaml` with CloudNativePG Cluster resource
  - Redis: Creates `redis-instance.yaml` with RedisFailover resource

### 3. Fixed File Structure
- Removed "voltron" subdirectory confusion
- All files now created directly under `manifests/`
- ApplicationSets look in the correct paths

## ⚠️ Current Issues

### GitHub Authentication Problem
- **Error**: "authentication required: Invalid username or password"
- **Cause**: GitHub token might be expired
- **Impact**: ArgoCD cannot sync from GitHub repository
- **Solution**: Need to update GitHub token

## 🏗️ Current Architecture

### How It Works Now

1. **Developer creates InfraForge claim**:
```yaml
apiVersion: platform.infraforge.io/v1
kind: InfraForge
metadata:
  name: test-finance
spec:
  tenant: finance
  environment: dev
  business:
    - name: backoffice
      enabled: true
      profile: dev
  platform:
    - name: vault
      enabled: true
      profile: dev
  operators:
    - name: postgresql
      enabled: true
```

2. **Kratix processes the claim**:
   - Triggers the infraforge-generator pipeline
   - Generator reads the claim and creates appropriate files

3. **Generator creates files**:
   ```
   manifests/
   ├── apps/
   │   └── dev/
   │       ├── business-apps/
   │       │   ├── backoffice/
   │       │   │   ├── Chart.yaml      # Helm chart definition
   │       │   │   └── values.yaml     # Helm values (replica, resources, etc)
   │       │   └── nginx/
   │       │       ├── Chart.yaml
   │       │       └── values.yaml
   │       └── platform-apps/
   │           └── vault/
   │               ├── Chart.yaml      # Points to HashiCorp Vault chart
   │               └── values.yaml     # Vault configuration
   ├── operators/
   │   └── dev/
   │       ├── postgresql/
   │       │   └── postgresql-cluster.yaml  # CloudNativePG Cluster CR
   │       └── redis/
   │           └── redis-instance.yaml      # Redis Failover CR
   └── appsets/
       └── dev/
           ├── business-appset.yaml
           ├── platform-appset.yaml
           └── operator-appset.yaml
   ```

4. **Kratix syncs to GitHub** (currently failing due to auth)

5. **ArgoCD ApplicationSets**:
   - Scan the directories for matching files
   - Create Applications for each discovered component
   - Deploy them to Kubernetes

## 📝 What Generator Does

### For Business Apps (backoffice, nginx, etc.)
Creates Helm-compatible structure:
- `Chart.yaml`: Defines the chart metadata
- `values.yaml`: Contains configuration like:
  - Replica count (based on profile: dev=1, standard=2, production=3)
  - Resources (CPU/memory limits)
  - Ingress configuration
  - Service configuration

### For Platform Apps (vault, istio, etc.)
Creates Helm chart that references upstream charts:
- `Chart.yaml`: Points to official Helm repository
- `values.yaml`: Configures the platform service

### For Operators (postgresql, redis, etc.)
Creates Custom Resources that operators will process:
- PostgreSQL: `Cluster` resource for CloudNativePG operator
- Redis: `RedisFailover` resource for Redis operator

## ❌ Why Apps Not Created Yet

1. **GitHub Authentication Failed**: ArgoCD cannot read from GitHub
2. **ApplicationSets Cannot Scan**: Without GitHub access, they can't find the files
3. **No Applications Generated**: ApplicationSets need to read files to generate Applications

## ✅ What's Working

1. **Kratix**: Processing claims successfully ✓
2. **Generator**: Creating correct file structure ✓
3. **Work Objects**: Ready and containing all files ✓
4. **ApplicationSets**: Deployed but waiting for GitHub access
5. **ArgoCD**: Running and ready

## 🔧 To Fix

Just need to update the GitHub token in the secret, then everything will work automatically.