# Percona MongoDB Profile Configurations

Bu dizindeki dosyalar, farklı environment'lar için profile tanımlarını içerir.

## Nasıl Çalışır?

Profile seçimi InfraForge claim'de yapılır:
```yaml
services:
  - name: my-db
    type: mongodb
    profile: production  # dev | standard | production
```

Generator, `cr.yaml.tmpl` template'indeki Go template logic ile profile'a göre değerleri set eder.

## Profile Dosyaları

- **dev.yaml**: Development environment konfigürasyonu
- **standard.yaml**: Staging/Pre-production konfigürasyonu  
- **production.yaml**: Production environment konfigürasyonu

Bu dosyalar:
1. Referans amaçlıdır - her profile'da hangi özellikler var gösterir
2. Gelecekte profile-based override sistemi eklenirse kullanılabilir
3. Dokümantasyon görevi görür

## Profile Özellikleri

### Development (dev.yaml)
- Single node (HA yok)
- Minimal resource (0.5-1 CPU, 1-2Gi RAM)
- 10Gi standard storage
- Backup/Monitoring disabled
- 1K max connections

### Standard (standard.yaml)
- 3 node ReplicaSet
- Moderate resource (1-2 CPU, 2-4Gi RAM)
- 50Gi SSD storage
- Daily + Weekly backup to MinIO
- Basic PMM monitoring
- 10K max connections

### Production (production.yaml)
- 5 primary + 2 non-voting nodes
- High resource (2-4 CPU, 4-8Gi RAM)
- 200Gi SSD storage
- Continuous backup with PITR
- Advanced monitoring
- Sharding enabled
- TLS required + Encryption at rest
- 65K max connections