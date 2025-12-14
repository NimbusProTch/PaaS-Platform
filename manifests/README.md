# InfraForge Manifests

Bu klasör InfraForge platform tarafından otomatik olarak yönetilir.

## Klasör Yapısı

```
manifests/
├── argocd/           # ArgoCD Applications (otomatik oluşturulur)
├── operators/        # Kubernetes operatörleri (MongoDB, PostgreSQL, vb.)
├── apps/             # Uygulama deploymentları
│   └── <tenant>/     # Tenant namespace
│       └── <env>/    # Environment (dev/staging/prod)
└── platform/         # Platform altyapı bileşenleri
```

## Kullanım

Bu klasördeki tüm dosyalar Kratix tarafından otomatik oluşturulur.
Değişiklikler InfraForge CRs üzerinden yapılmalıdır.