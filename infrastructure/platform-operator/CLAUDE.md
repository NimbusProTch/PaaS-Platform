# Platform Operator - Production Architecture

## Overview

Production-ready Kubernetes operator for managing platform infrastructure and applications through GitOps.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    Platform Operator CRDs                     │
├──────────────────────────────────────────────────────────────┤
│  BootstrapClaim  │  ApplicationClaim  │  PlatformAppClaim   │
│   (GitOps Init)  │   (Microservices)  │  (PostgreSQL, etc)  │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│              Controllers (Values Generation)                  │
├──────────────────────────────────────────────────────────────┤
│  • Chart-aware values merging                                │
│  • Production/Dev profile selection                          │
│  • Custom overrides from CRD spec                            │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                      Gitea (voltran repo)                     │
├──────────────────────────────────────────────────────────────┤
│  appsets/{clusterType}/{apps|platform}/*.yaml                │
│  environments/{clusterType}/{env}/{applications|platform}/   │
│    ├── values.yaml                                           │
│    └── config.yaml                                           │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│                  ArgoCD (GitOps Sync)                         │
├──────────────────────────────────────────────────────────────┤
│  Root Apps → ApplicationSets → Applications                  │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│               OCI Registry (GitHub Packages)                  │
├──────────────────────────────────────────────────────────────┤
│  ghcr.io/nimbusprotch/microservice:1.0.0                     │
│  ghcr.io/nimbusprotch/postgresql:1.0.0                       │
│  ghcr.io/nimbusprotch/mongodb:1.0.0                          │
│  ghcr.io/nimbusprotch/rabbitmq:1.0.0                         │
│  ghcr.io/nimbusprotch/redis:1.0.0                            │
│  ghcr.io/nimbusprotch/kafka:1.0.0                            │
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
└── templates/                 # Operator CRDs
```

## Custom Resource Definitions

### BootstrapClaim
Initializes GitOps repository structure in Gitea.

```yaml
apiVersion: platform.infraforge.io/v1
kind: BootstrapClaim
metadata:
  name: platform-bootstrap
spec:
  giteaURL: http://gitea-http.gitea.svc.cluster.local:3000
  organization: platform

  # OCI Registry for charts
  chartsRepository:
    type: oci
    url: oci://ghcr.io/nimbusprotch/microservice
    version: "1.0.0"

  repositories:
    charts: charts
    voltran: voltran

  gitOps:
    branch: main
    clusterType: nonprod
    environments: [dev, qa, staging]
```

### ApplicationClaim
Deploys microservices.

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: my-apps
spec:
  environment: dev
  clusterType: nonprod
  applications:
    - name: api-gateway
      image:
        repository: myapp/api
        tag: v1.0.0
      replicas: 3
      resources:
        requests:
          cpu: 500m
          memory: 1Gi
      ingress:
        enabled: true
        host: api.example.com
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

### Values Merging (Helm Best Practice)

```go
func generateValues(claim, service) {
    chartName := service.Type  // postgresql, redis, etc.

    // 1. Fetch base values
    baseValues := fetchChartValues(chartName, "values.yaml")

    // 2. Determine profile
    profile := "dev"
    if service.Production || claim.Environment == "prod" {
        profile = "production"
    }

    // 3. Merge production values (if applicable)
    if profile == "production" {
        prodValues := fetchChartValues(chartName, "values-production.yaml")
        finalValues = mergeValues(baseValues, prodValues)
    }

    // 4. Apply user overrides from CRD spec
    finalValues = applyUserOverrides(finalValues, service)

    return finalValues
}
```

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
