# Helm Charts

Production-ready Helm charts for platform operator.

## Published Charts

All charts are published to GitHub Container Registry (OCI):

- `oci://ghcr.io/nimbusprotch/microservice` - Generic microservice application chart
- `oci://ghcr.io/nimbusprotch/postgresql` - CloudNative-PG PostgreSQL Cluster
- `oci://ghcr.io/nimbusprotch/mongodb` - MongoDB Community Operator
- `oci://ghcr.io/nimbusprotch/rabbitmq` - RabbitMQ Cluster Operator
- `oci://ghcr.io/nimbusprotch/redis` - Redis Cluster (OT-CONTAINER-KIT)
- `oci://ghcr.io/nimbusprotch/kafka` - Strimzi Kafka Operator

## Chart Structure

Each chart follows production-ready patterns:

```
charts/<chart-name>/
├── Chart.yaml                 # Chart metadata & version
├── values.yaml                # Base values (dev defaults)
├── values-production.yaml     # Production overrides
└── templates/                 # Kubernetes manifests
```

## Usage

### Pull & Install

```bash
# Pull chart
helm pull oci://ghcr.io/nimbusprotch/postgresql --version 1.0.0

# Install with dev defaults
helm install mydb oci://ghcr.io/nimbusprotch/postgresql --version 1.0.0

# Install with production values
helm install mydb oci://ghcr.io/nimbusprotch/postgresql \
  --version 1.0.0 \
  -f values-production.yaml
```

### Platform Operator Usage

The platform operator automatically pulls these charts based on CRD definitions:

```yaml
apiVersion: platform.infraforge.io/v1
kind: PlatformApplicationClaim
spec:
  environment: prod
  services:
    - type: postgresql
      name: orders-db
      production: true  # Uses values-production.yaml
      storage:
        size: 200Gi
```

## Operators Required

Platform charts require these Kubernetes operators to be installed:

- **CloudNative-PG:** PostgreSQL clusters
- **MongoDB Community Operator:** MongoDB replica sets
- **RabbitMQ Cluster Operator:** RabbitMQ clusters
- **Redis Operator (OT-CONTAINER-KIT):** Redis clusters
- **Strimzi:** Kafka clusters

See `infrastructure/operators/` for installation manifests.

## Development

### Local Testing

```bash
# Lint chart
helm lint charts/postgresql

# Template chart
helm template test charts/postgresql

# Template with production values
helm template test charts/postgresql -f charts/postgresql/values-production.yaml

# Dry-run install
helm install --dry-run test charts/postgresql
```

### Versioning

Each chart has independent semantic versioning in `Chart.yaml`:

```yaml
version: 1.0.0  # Chart version
appVersion: "16"  # Application version
```

Update version when making changes, then push to main - GitHub Actions will automatically publish.

## Architecture

Platform Operator workflow:

1. **Developer** creates ApplicationClaim or PlatformApplicationClaim
2. **Operator** determines chart name & version from claim spec
3. **Operator** merges base values + production values + user overrides
4. **Operator** writes final values to Gitea voltran repository
5. **ArgoCD** syncs from Gitea and installs using OCI chart reference

```
User Claim → Operator → Values Merge → Gitea → ArgoCD → OCI Chart Pull → K8s
```

## Support

For issues or questions, open an issue on GitHub.
