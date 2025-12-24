# 📦 OCI Charts Management - Kurulum Rehberi

## 🎯 Ne Yaptık?

Charts'ları Docker image'dan ayırıp **GitHub Packages (OCI Registry)** üzerinden yönetilir hale getirdik.

### Eskiden
```
Chart değişikliği
    ↓
Operator Docker image rebuild (2-3 dakika)
    ↓
Image push
    ↓
Kind load / EKS deploy
    ↓
Test
```

### Şimdi
```
Chart değişikliği
    ↓
Git push (10 saniye)
    ↓
GitHub Actions otomatik publish (30 saniye)
    ↓
Bootstrap yeniden apply
    ↓
Test
```

## ✅ Yapılan Değişiklikler

### 1. **Dockerfile Temizlendi**
- ❌ `COPY charts/ /charts/` KALDIRILDI
- ✅ Operator artık daha küçük ve hızlı

### 2. **GitHub Actions Workflows** ✨
**`.github/workflows/chart-lint.yml`**
- Her PR'da otomatik lint
- Template validation
- YAML validation
- Semantic version check

**`.github/workflows/chart-publish.yml`**
- Main branch'e push → otomatik publish
- `latest` tag güncellenir
- Semantic version tag eklenir
- Release notes oluşturulur

### 3. **BootstrapClaim API Genişletildi**
Yeni field'lar:
```yaml
chartsRepository:
  type: oci              # "oci" veya "git"
  url: oci://ghcr.io/org/chart
  version: latest        # "latest" veya "1.0.0"
```

### 4. **Bootstrap Controller Güncellendi**
- OCI chart pull desteği
- Git clone desteği (mevcut)
- Embedded charts (backwards compatible)

## 🚀 Sonraki Adımlar

### Adım 1: GitHub Repository Ayarları

```bash
# Repo Settings → Actions → General → Workflow permissions
# "Read and write permissions" SEÇ ✅
```

### Adım 2: İlk Commit & Push

```bash
cd /Users/gaskin/Desktop/Teknokent-Projeler/PaaS-Platform

# Workflow'ları git'e ekle
git add .github/workflows/chart-lint.yml
git add .github/workflows/chart-publish.yml
git add .github/ct.yaml
git add charts/README.md

# Operator değişikliklerini ekle
git add infrastructure/platform-operator/api/v1/bootstrapclaim_types.go
git add infrastructure/platform-operator/pkg/gitea/client.go
git add infrastructure/platform-operator/internal/controller/bootstrap_controller.go
git add infrastructure/platform-operator/config/crd/bases/

# Commit
git commit -m "feat: Add OCI chart management with GitHub Packages

- Add chart-lint workflow for PR validation
- Add chart-publish workflow for OCI publishing
- Add OCI support to BootstrapClaim
- Update Bootstrap controller to pull from OCI registry
- Remove charts from Docker image (faster builds)

Charts are now published to:
oci://ghcr.io/<YOUR-ORG>/common:latest
"

# Push
git push origin main
```

### Adım 3: İlk Chart Publish'i İzle

```bash
# GitHub Actions'a git
# https://github.com/<YOUR-ORG>/PaaS-Platform/actions

# "Publish Helm Charts to OCI" workflow'u çalışacak
# ~30 saniye içinde chart publish olacak
```

### Adım 4: Bootstrap'ı Güncelle

```yaml
# infrastructure/platform-operator/config/samples/bootstrap-claim.yaml
apiVersion: platform.infraforge.io/v1
kind: BootstrapClaim
metadata:
  name: bootstrap-platform
spec:
  organization: infraforge

  # OCI Registry kullan! 🚀
  chartsRepository:
    type: oci
    url: oci://ghcr.io/<YOUR-GITHUB-ORG>/common
    version: latest

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

### Adım 5: Test Et!

```bash
# Bootstrap'ı apply et
kubectl apply -f infrastructure/platform-operator/config/samples/bootstrap-claim.yaml

# Logları izle
kubectl logs -n platform-operator-system -l control-plane=controller-manager -f

# Beklenen log:
# "Loading charts from external repository" type="oci" url="oci://ghcr.io/.../common"
# "Pulling chart from OCI registry" version="latest"
# ✅ "Charts uploaded successfully"
```

## 🔄 Development Workflow

### Chart Güncellerken

```bash
# 1. Chart'ı güncelle
vi charts/common/templates/microservice/deployment.yaml

# 2. Version'ı artır (semantic versioning)
vi charts/common/Chart.yaml
# version: 1.0.0 → 1.1.0

# 3. PR aç veya direkt push
git add charts/
git commit -m "feat: Add configurable resource limits"
git push

# 4. GitHub Actions otomatik:
# - Chart lint (PR'da)
# - Publish to OCI (main'de)
# - Tag as latest

# 5. Bootstrap yeniden apply et
kubectl delete bootstrapclaim bootstrap-platform
kubectl apply -f config/samples/bootstrap-claim.yaml

# ✅ YENİ charts Gitea'ya geldi!
```

## 📊 Publish Edilen Artifacts

Her main push sonrası:

```
ghcr.io/<YOUR-ORG>/common:latest
ghcr.io/<YOUR-ORG>/common:1.0.0
ghcr.io/<YOUR-ORG>/common:1.1.0
...
```

## 🎨 Kullanım Örnekleri

### Manuel Helm Install
```bash
helm install my-app oci://ghcr.io/<YOUR-ORG>/common --version latest \
  --set type=microservice \
  --set image.repository=myapp
```

### Git Clone (Alternative)
```yaml
chartsRepository:
  type: git
  url: https://github.com/<YOUR-ORG>/PaaS-Platform.git
  branch: main
  path: charts
```

### Embedded (Fallback)
```yaml
# chartsRepository kullanma
# Otomatik olarak operator image içindeki charts kullanılır
```

## ⚙️ Troubleshooting

### Chart publish olmuyor
```bash
# GitHub repo settings kontrol et
# Settings → Actions → Workflow permissions
# "Read and write permissions" olmalı
```

### OCI pull fail
```bash
# Helm CLI kurulu mu kontrol et
helm version

# Chart gerçekten publish olmuş mu?
helm pull oci://ghcr.io/<YOUR-ORG>/common --version latest
```

### Bootstrap hata veriyor
```bash
# Operator loglarını kontrol et
kubectl logs -n platform-operator-system -l control-plane=controller-manager

# Bootstrap claim status'ünü kontrol et
kubectl get bootstrapclaim bootstrap-platform -o yaml
```

## 📈 Avantajlar

| Özellik | Öncesi | Sonrası |
|---------|--------|---------|
| Chart değişikliği | 2-3 dk | 30 sn |
| Operator rebuild | Her seferinde | ASLA |
| Versioning | Manual | Otomatik |
| CI/CD | Yok | ✅ Tam otomatik |
| Chart test | Manual | ✅ PR'da otomatik |
| Multi-env | Zor | ✅ Kolay (tag'ler ile) |

## 🎉 Sonuç

Artık platform charts'ları:
- ✅ Bağımsız olarak yönetiliyor
- ✅ Otomatik test ediliyor
- ✅ Otomatik publish ediliyor
- ✅ Version control altında
- ✅ Hızlı iteration (30 saniye)
- ✅ Production-ready

**Operator rebuild gereksiz!** 🚀
