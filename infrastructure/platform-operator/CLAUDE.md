# Platform Operator - Production Architecture

## Overview

Production-ready Kubernetes operator for managing platform infrastructure and applications through GitOps with OCI-based Helm charts.

## Architecture Flow

```
┌──────────────────────────────────────────────────────────────┐
│          GitHub Packages (OCI Registry)                       │
├──────────────────────────────────────────────────────────────┤
│  📦 Helm Charts (Templates - Stable, 6 charts):              │
│     • microservice:1.0.0    (Generic app deployment)         │
│     • postgresql:1.0.0      (CloudNative-PG)                 │
│     • mongodb:1.0.0         (MongoDB Operator)               │
│     • redis:1.0.0           (Redis Operator)                 │
│     • rabbitmq:1.0.0        (RabbitMQ Operator)              │
│     • kafka:1.0.0           (Strimzi Operator)               │
│                                                              │
│  📦 Docker Images (Apps - Dynamic, 100+ images):             │
│     • notification-service:v1.2.3                            │
│     • payment-service:v2.0.1                                 │
│     • user-service:v1.5.0                                    │
│     • ecommerce-platform:v1.0.0                              │
│     • ... (all microservices)                                │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                    Platform Operator CRDs                     │
├──────────────────────────────────────────────────────────────┤
│  BootstrapClaim  │  ApplicationClaim  │  PlatformAppClaim   │
│   (GitOps Init)  │   (Microservices)  │  (PostgreSQL, etc)  │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│              Controllers (Smart Values Merging)               │
├──────────────────────────────────────────────────────────────┤
│  1. Pull chart from OCI (temp, cached)                       │
│  2. Read base values.yaml                                    │
│  3. Merge values-production.yaml (if prod env)               │
│  4. Apply CRD custom overrides (image, replicas, etc)        │
│  5. Push ONLY final values.yaml to Gitea                     │
│  6. Generate ApplicationSet with OCI chart reference         │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│              Gitea (voltran repo - VALUES ONLY)               │
├──────────────────────────────────────────────────────────────┤
│  appsets/{clusterType}/{apps|platform}/*.yaml                │
│  environments/{clusterType}/{env}/applications/              │
│    notification-service/                                     │
│      ├── values.yaml (FINAL merged values)                   │
│      └── config.yaml (chart: microservice:1.0.0)             │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                  ArgoCD (GitOps Sync)                         │
├──────────────────────────────────────────────────────────────┤
│  Chart:  oci://ghcr.io/nimbusprotch/microservice:1.0.0      │
│  Values: voltran/environments/.../values.yaml                │
│  → Deploys: notification-service:v1.2.3                      │
└──────────────────────────────────────────────────────────────┘
```

## Helm Charts (Production-Ready)

All charts use official Kubernetes operators:

### Application Charts
- **microservice** - Generic app deployment (Deployment, Service, Ingress, HPA, ServiceAccount)

### Platform Charts (Operator-Based)
- **postgresql** - CloudNative-PG (CNCF Sandbox)
- **mongodb** - MongoDB Community Operator (Official)
- **rabbitmq** - RabbitMQ Cluster Operator (VMware)
- **redis** - Redis Operator (OT-CONTAINER-KIT)
- **kafka** - Strimzi Kafka Operator (CNCF Sandbox)

### Chart Structure
```
charts/<name>/
├── Chart.yaml                 # Metadata & version
├── values.yaml                # Base (dev defaults)
├── values-production.yaml     # Production overrides
└── templates/                 # K8s manifests / Operator CRDs
```

## Package Strategy

### Two Package Types

**1. Helm Chart Packages (6 charts - Stable, OCI)**
- Published to: `ghcr.io/nimbusprotch/<chart-name>:<version>`
- Changed: Rarely (when template logic changes)
- Versioning: SemVer (1.0.0, 1.0.1, 1.1.0...)
- Examples:
  - `ghcr.io/nimbusprotch/microservice:1.0.0`
  - `ghcr.io/nimbusprotch/postgresql:1.0.0`

**2. Docker Image Packages (100+ apps - Dynamic, OCI)**
- Published to: `ghcr.io/nimbusprotch/<service-name>:<version>`
- Changed: Frequently (every deployment)
- Versioning: SemVer with 'v' prefix (v1.0.0, v1.2.3, v2.0.0...)
- Examples:
  - `ghcr.io/nimbusprotch/notification-service:v1.2.3`
  - `ghcr.io/nimbusprotch/payment-service:v2.0.1`

### Key Principle
- **Charts** = Templates (reusable across 100s of apps)
- **Images** = Application code (unique per microservice)
- **Values** = Configuration (environment-specific, stored in Gitea)

## Custom Resource Definitions

### BootstrapClaim
Initializes GitOps repository structure in Gitea.

**Behavior:**
- Creates Gitea organization and repositories (voltran, charts - optional)
- Creates GitOps folder structure (appsets, environments)
- **DOES NOT** push charts to Gitea (charts live in OCI registry only)
- Generates ArgoCD root applications and ApplicationSet scaffolding

```yaml
apiVersion: platform.infraforge.io/v1
kind: BootstrapClaim
metadata:
  name: platform-bootstrap
spec:
  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: platform

  # Optional: OCI Registry reference (for documentation only)
  # Operator does NOT pull/push charts during bootstrap
  chartsRepository:
    type: oci
    url: oci://ghcr.io/nimbusprotch
    version: "1.0.0"

  repositories:
    voltran: voltran  # GitOps manifests repo (REQUIRED)

  gitOps:
    branch: main
    clusterType: nonprod
    environments: [dev, qa, staging]
```

### ApplicationClaim
Deploys microservices with smart values merging.

**Behavior:**
1. Pulls Helm chart from OCI registry (`microservice:1.0.0`)
2. Reads `values.yaml` (dev defaults)
3. If `environment: prod` → merges `values-production.yaml`
4. Merges CRD spec overrides (image, replicas, resources, etc.)
5. Pushes **ONLY** final `values.yaml` to Gitea voltran repo
6. Generates ApplicationSet with OCI chart reference

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: my-services
spec:
  environment: prod
  clusterType: prod
  applications:
    - name: notification-service
      chart:
        name: microservice        # Common chart for all apps
        version: "1.0.0"
      image:
        repository: ghcr.io/nimbusprotch/notification-service
        tag: v1.2.3              # App-specific version
      replicas: 5
      resources:
        requests:
          cpu: 500m
          memory: 512Mi
      ingress:
        enabled: true
        host: notifications.example.com

    - name: payment-service
      chart:
        name: microservice        # Same chart, different app
        version: "1.0.0"
      image:
        repository: ghcr.io/nimbusprotch/payment-service
        tag: v2.0.1
      replicas: 10
```

### PlatformApplicationClaim
Deploys platform infrastructure.

```yaml
apiVersion: platform.infraforge.io/v1
kind: PlatformApplicationClaim
metadata:
  name: platform-services
spec:
  environment: prod
  clusterType: prod
  services:
    - type: postgresql
      name: orders-db
      production: true  # Uses values-production.yaml
      storage:
        size: 200Gi
      backup:
        s3Bucket: s3://my-backups

    - type: redis
      name: cache
      production: true
      storage:
        size: 50Gi
```

## Controller Logic

### Values Merging (Smart, OCI-based)

```go
func generateValues(claim, app) {
    chartName := app.Chart.Name     // e.g., "microservice"
    chartVersion := app.Chart.Version // e.g., "1.0.0"

    // 1. Pull chart from OCI registry (temp, cached)
    chartPath := pullOCIChart("oci://ghcr.io/nimbusprotch/"+chartName, chartVersion)

    // 2. Read base values.yaml
    baseValues := readFile(chartPath + "/values.yaml")

    // 3. Determine profile
    profile := "dev"
    if claim.Environment == "prod" || claim.ClusterType == "prod" {
        profile = "production"
    }

    // 4. Merge production values (if applicable)
    finalValues := baseValues
    if profile == "production" {
        prodFile := chartPath + "/values-production.yaml"
        if fileExists(prodFile) {
            prodValues := readFile(prodFile)
            finalValues = mergeValues(baseValues, prodValues)  // Deep merge
        }
    }

    // 5. Apply CRD custom overrides (image, replicas, resources, env, etc.)
    finalValues = applyUserOverrides(finalValues, app)

    return finalValues
}
```

**Key Points:**
- Charts are **pulled from OCI**, NOT from Gitea
- Charts are **cached locally**, not re-downloaded every reconcile
- Only **final values.yaml** is pushed to Gitea
- ArgoCD references **OCI chart** directly

### GitOps Directory Structure

```
voltran/
├── root-apps/{clusterType}/
│   ├── {clusterType}-apps-rootapp.yaml
│   └── {clusterType}-platform-rootapp.yaml
│
├── appsets/{clusterType}/
│   ├── apps/{env}-appset.yaml
│   └── platform/{env}-platform-appset.yaml
│
└── environments/{clusterType}/{env}/
    ├── applications/{app}/
    │   ├── values.yaml
    │   └── config.yaml
    └── platform/{service}/
        └── values.yaml
```

## CI/CD (GitHub Actions)

### Workflow: `chart-publish.yml`

Triggers on push to `charts/**`:

1. Package each chart with its own version
2. Publish to `ghcr.io/nimbusprotch/{chart-name}:{version}`
3. Generate release notes

Each chart version is independent:
```bash
ghcr.io/nimbusprotch/microservice:1.0.0
ghcr.io/nimbusprotch/postgresql:1.0.0
ghcr.io/nimbusprotch/mongodb:1.0.0
```

## Development Workflow

### 1. Update Chart
```bash
vi charts/postgresql/templates/cluster.yaml
vi charts/postgresql/Chart.yaml  # Bump version: 1.0.0 → 1.0.1
```

### 2. Push to Main
```bash
git add charts/
git commit -m "feat: Add PostgreSQL backup configuration"
git push
```

### 3. GitHub Actions Auto-Publishes
- Packages `postgresql-1.0.1.tgz`
- Pushes to `oci://ghcr.io/nimbusprotch/postgresql:1.0.1`

### 4. Update CRD
```yaml
chartsRepository:
  url: oci://ghcr.io/nimbusprotch/postgresql
  version: "1.0.1"  # New version
```

### 5. Apply CRD
```bash
kubectl apply -f bootstrap-claim.yaml
```

## Testing

### Local Chart Testing
```bash
# Lint
helm lint charts/postgresql

# Template (dev)
helm template test charts/postgresql

# Template (prod)
helm template test charts/postgresql \
  -f charts/postgresql/values-production.yaml

# Install dry-run
helm install --dry-run test charts/postgresql
```

### Operator Testing
```bash
# Run operator locally
export GITEA_TOKEN=<token>
go run cmd/manager/main.go \
  --gitea-url=http://localhost:30300 \
  --gitea-username=admin

# Apply claims
kubectl apply -f bootstrap-claim.yaml
kubectl apply -f app-claim.yaml
kubectl apply -f platform-claim.yaml
```

## Production Deployment

### Prerequisites
1. Install Kubernetes operators:
   - CloudNative-PG
   - MongoDB Community Operator
   - RabbitMQ Cluster Operator
   - Redis Operator
   - Strimzi Kafka Operator

2. Install ArgoCD

3. Deploy Gitea

4. Create Gitea admin user & token

### Deploy Operator
```bash
kubectl apply -f infrastructure/platform-operator/config/crd/
kubectl apply -f infrastructure/platform-operator/config/manager/
```

### Bootstrap Platform
```bash
kubectl apply -f infrastructure/platform-operator/bootstrap-claim.yaml
```

## Key Features

✅ **Production-Ready Operators** - CNCF/Official operators only
✅ **GitOps-Native** - ArgoCD ApplicationSets
✅ **OCI Chart Distribution** - GitHub Packages
✅ **Environment Profiles** - Dev/Prod values separation
✅ **Chart-Aware Values** - Merge base + prod + custom
✅ **Independent Versioning** - Each chart versions independently
✅ **Automated Publishing** - GitHub Actions
✅ **Clean Controller Code** - No hardcoded values
✅ **Testable** - Helm lint, template, dry-run
✅ **Extensible** - Add new charts easily

## Next Steps

1. Implement controller values merging logic
2. Add operator installation manifests
3. Enhance CRD validation with OpenAPI schemas
4. Add metrics and monitoring
5. Write comprehensive E2E tests

## Support

For issues or questions, see GitHub Issues.
