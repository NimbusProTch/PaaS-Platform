# Platform Helm Charts

Bu repository platform ve mikroservisler için Helm chart templatelerini içerir.

## 📦 Publish Edilen Packages

Her main branch'e push otomatik olarak GitHub Packages (OCI Registry) üzerine publish edilir.

**Published Chart:**
```
oci://ghcr.io/nimbusprotch/common:latest
oci://ghcr.io/nimbusprotch/common:<version>
```

## 🚀 Kullanım

### Manuel Helm Install

```bash
# Latest version pull
helm pull oci://ghcr.io/nimbusprotch/common --version latest

# Specific version pull
helm pull oci://ghcr.io/nimbusprotch/common --version 1.0.0

# Install
helm install my-app oci://ghcr.io/nimbusprotch/common --version latest \
  --set type=microservice \
  --set image.repository=myapp \
  --set image.tag=latest
```

### Platform Operator Bootstrap

```yaml
apiVersion: platform.infraforge.io/v1
kind: BootstrapClaim
metadata:
  name: bootstrap-platform
spec:
  organization: infraforge

  # OCI Registry (ÖNERİLEN - Her push'ta latest güncellenir)
  chartsRepository:
    type: oci
    url: oci://ghcr.io/nimbusprotch/common
    version: "2.0.0"  # veya "1.0.0" gibi spesifik version

  repositories:
    charts: charts
    voltran: voltran

  gitOps:
    branch: main
    clusterType: nonprod
    environments:
      - dev
      - qa
      - sandbox
```

## 📝 Chart Types

### Mikroservis (type: microservice)
Standart microservice deployment için.

**Değerler:**
```yaml
type: microservice
image:
  repository: myapp
  tag: latest
replicaCount: 2
resources:
  requests:
    cpu: 100m
    memory: 128Mi
env:
  - name: NODE_ENV
    value: production
```

### Platform Services

#### PostgreSQL (type: postgresql)
CloudNativePG operator ile HA PostgreSQL.

```yaml
type: postgresql
postgresql:
  instances: 3
  storage:
    size: 20Gi
```

#### Redis (type: redis)
Redis Sentinel/Cluster.

```yaml
type: redis
redis:
  mode: sentinel
  sentinel:
    replicas: 3
  redis:
    replicas: 3
```

#### RabbitMQ (type: rabbitmq)
```yaml
type: rabbitmq
rabbitmq:
  replicas: 3
  storage:
    size: 10Gi
```

#### MongoDB (type: mongodb)
```yaml
type: mongodb
mongodb:
  type: ReplicaSet
  members: 3
```

#### Elasticsearch (type: elasticsearch)
```yaml
type: elasticsearch
elasticsearch:
  nodeSets:
    master:
      count: 3
```

## 🔄 Development Workflow

### 1. Chart Değiştir
```bash
# Template güncelle
vi charts/common/templates/microservice/deployment.yaml

# Values güncelle
vi charts/common/values.yaml
```

### 2. Test Et
```bash
# Lint
helm lint charts/common/

# Template test
helm template test charts/common/ \
  --set type=microservice \
  --set image.repository=test
```

### 3. PR Aç
```bash
git checkout -b feat/update-deployment
git add charts/
git commit -m "feat: Add resource limits"
git push
```

**GitHub Actions otomatik:**
- ✅ Helm lint çalışır
- ✅ Template validation
- ✅ YAML validation

### 4. Merge → Otomatik Publish
Main branch'e merge olunca:
- ✅ Chart package edilir
- ✅ GitHub Packages'a push edilir
- ✅ `latest` tag güncellenir
- ✅ Semantic version tag eklenir

## 📊 Versioning

`charts/common/Chart.yaml` içindeki version semantic versioning kullanır:

```yaml
version: 1.2.3  # MAJOR.MINOR.PATCH
```

- **MAJOR**: Breaking changes
- **MINOR**: Yeni feature (backwards compatible)
- **PATCH**: Bug fix

## 🎯 CI/CD

### PR Workflow (`.github/workflows/chart-lint.yml`)
- Helm lint
- Template validation
- YAML validation
- Version check

### Publish Workflow (`.github/workflows/chart-publish.yml`)
- Chart package
- OCI push (versioned)
- OCI push (latest)
- Release notes

## 🔐 Permissions

GitHub Packages publish etmek için:
1. Repo Settings → Actions → Workflow permissions
2. "Read and write permissions" seç
3. Save

## 📚 Daha Fazla Bilgi

- [Helm OCI Support](https://helm.sh/docs/topics/registries/)
- [GitHub Packages](https://docs.github.com/en/packages)
- [CloudNativePG](https://cloudnative-pg.io/)
- [Platform Operator Docs](../infrastructure/platform-operator/README.md)
