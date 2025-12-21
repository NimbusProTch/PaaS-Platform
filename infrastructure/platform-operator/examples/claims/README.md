# ApplicationClaim Examples

Bu dizin, platform üzerinde uygulama deploy etmek için ApplicationClaim örnekleri içerir.

## Örnekler

### 1. GitHub Release ile Deploy (`ecommerce-claim-ghcr.yaml`)

GitHub Container Registry'deki versiyonlanmış image'ları kullanır:

```yaml
applications:
  - name: ecommerce-platform
    serviceName: ecommerce-platform  # GitHub release service adı
    version: v1.0.0                  # GitHub release version
    replicas: 2
```

**Kullanım:**
```bash
kubectl apply -f examples/claims/ecommerce-claim-ghcr.yaml
```

### 2. Basit Deployment (`ecommerce-claim-simple.yaml`)

Doğrudan image URL'i ile deploy:

```yaml
applications:
  - name: my-app
    image: ghcr.io/nimbusprotech/ecommerce-platform:v1.0.0
    replicas: 2
```

## ApplicationClaim Yapısı

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: my-application
  namespace: default
spec:
  environment: dev | staging | prod

  owner:
    team: team-name
    email: team@example.com
    slack: "#channel"

  applications:
    - name: app-name
      # Seçenek 1: GitHub Release (önerilen)
      serviceName: service-name
      version: v1.0.0

      # Seçenek 2: Direkt image
      image: ghcr.io/org/image:tag

      replicas: 2
      ports:
        - name: http
          port: 8080

      healthCheck:
        path: /health
        port: 8080

      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "500m"
          memory: "512Mi"

      env:
        - name: ENV_VAR
          value: "value"

  components:
    - type: postgresql
      name: database
      version: "15"
      size: small | medium | large
```

## Deployment Workflow

1. **Tag Oluştur**: `git tag ecommerce-platform-v1.0.0 && git push --tags`
2. **GitHub Actions**: Otomatik image build ve release oluşturur
3. **ApplicationClaim Deploy**: `kubectl apply -f claim.yaml`
4. **Operator**: GitHub'dan image URL'i çeker ve deploy eder
5. **ArgoCD**: GitOps ile sync eder

## Daha Fazla Bilgi

- [ApplicationClaim CRD](../../config/crd/bases/platform.infraforge.io_applicationclaims.yaml)
- [Operator README](../../README.md)
