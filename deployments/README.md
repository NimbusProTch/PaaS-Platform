# Deployments

Bu dizin, gerçek production/staging/dev ortamlarına yapılan deployment'ları içerir.

## Dizin Yapısı

```
deployments/
├── dev/              # Development ortamı
├── staging/          # Staging ortamı
└── prod/             # Production ortamı
```

## Kullanım

Her ortam kendi ApplicationClaim'lerini içerir:

```bash
# Development'a deploy
kubectl apply -f deployments/dev/

# Staging'e deploy
kubectl apply -f deployments/staging/

# Production'a deploy
kubectl apply -f deployments/prod/
```

## GitOps Workflow

Bu dizin ArgoCD tarafından izlenir:

1. ApplicationClaim'i ilgili ortam dizinine commit et
2. Git'e push et
3. ArgoCD otomatik algılar ve deploy eder
4. Operator manifestleri generate eder
5. Uygulama cluster'a deploy edilir

## Örnek Dosya

`deployments/dev/ecommerce.yaml`:

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: ecommerce-dev
  namespace: default
spec:
  environment: dev
  owner:
    team: ecommerce-team
    email: ecommerce@company.com
  applications:
    - name: ecommerce-platform
      serviceName: ecommerce-platform
      version: v1.0.0
      replicas: 1
```

## Best Practices

- ✅ Her ortam için ayrı namespace kullan
- ✅ Environment-specific resource limits belirle
- ✅ Version tag'lerini kullan (`:latest` değil)
- ✅ PR ile deploy et (direct push değil)
- ✅ Staging'de test et, sonra production'a geç
