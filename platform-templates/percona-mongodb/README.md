# Percona MongoDB Templates for InfraForge

Enterprise-grade MongoDB deployment using Percona Server for MongoDB Operator.

## Yapı

```
percona-mongodb/
├── cr.yaml.tmpl       # PerconaServerMongoDB CR template
└── profiles/          # Profile dokümantasyonu
    └── README.md
```

## Nasıl Çalışır?

1. **User** InfraForge claim oluşturur
2. **Kratix** CR template'i işler ve Git'e yazar
3. **ArgoCD** CR'ı sync eder
4. **Percona Operator** CR'ı görür ve:
   - StatefulSet oluşturur
   - Service'leri oluşturur
   - PVC'leri oluşturur
   - Backup job'ları oluşturur

## Profile Özellikleri

### 🟢 Development (`profile: dev`)
- **Replicas**: 1 node (no HA)
- **Resources**: 0.5-1 CPU, 1-2Gi RAM
- **Storage**: 10Gi standard disk
- **Backup**: ❌ Disabled
- **Monitoring**: ❌ Disabled
- **TLS**: ❌ Disabled
- **Connection Pool**: 1,000 max connections

### 🟡 Standard (`profile: standard`)
- **Replicas**: 3 node ReplicaSet
- **Resources**: 1-2 CPU, 2-4Gi RAM
- **Storage**: 50Gi fast-ssd
- **Backup**: ✅ Daily + Weekly to MinIO
- **Monitoring**: ✅ PMM enabled
- **TLS**: ✅ Preferred
- **Connection Pool**: 10,000 max connections
- **OpLog**: 5GB size
- **Cache**: 2GB WiredTiger cache

### 🔴 Production (`profile: production`)
- **Replicas**: 5 node + 2 non-voting (read scaling)
- **Resources**: 2-4 CPU, 4-8Gi RAM (primary), 1-2 CPU, 2-4Gi RAM (non-voting)
- **Storage**: 200Gi fast-ssd
- **Backup**: ✅ 
  - Incremental every 30min
  - Daily full backup
  - Weekly to NFS
  - Monthly archive
  - PITR enabled (1 hour oplog)
- **Monitoring**: ✅ PMM Advanced with custom settings
- **TLS**: ✅ Required
- **Encryption**: ✅ At rest encryption
- **Sharding**: ✅ 3 config servers + 3 mongos
- **Connection Pool**: 65,536 max connections
- **OpLog**: 10GB size
- **Cache**: 4GB WiredTiger cache
- **Compression**: zstd for better performance
- **Advanced**:
  - Concurrent transactions: 128 read/write
  - TTL monitor optimization
  - Query profiling for all operations
  - Log aggregation sidecar

## Generasyon Örneği

Input (InfraForge claim):
```yaml
services:
  - name: customer-db
    type: mongodb
    profile: production
```

Output:
```
workloads/demo-team-prod/mongodb/
├── customer-db-mongodb.yaml    # PerconaServerMongoDB CR
├── customer-db-secrets.yaml    # User credentials
└── customer-db-encryption-key.yaml  # Encryption key (prod only)
```

## Backup Detayları

### Standard Profile
- **Schedule**: Daily 02:00, Weekly Sunday 03:00
- **Retention**: 7 daily, 4 weekly
- **Storage**: MinIO (S3 compatible)
- **Compression**: gzip

### Production Profile
- **Incremental**: Every 30 minutes (48 retained)
- **Full Daily**: 02:00 (7 retained)
- **Weekly NFS**: Sunday 03:00 (8 retained)
- **Monthly Archive**: 1st day 04:00 (12 retained)
- **PITR**: 60 minute oplog window
- **Compression**: zstd level 6-9
- **Dual Storage**: MinIO (primary) + NFS (secondary)

## Monitoring Detayları

### PMM (Percona Monitoring and Management)
- **Dev**: Disabled
- **Standard**: Basic monitoring
- **Production**: 
  - Advanced query analytics
  - 2000 table stats limit
  - Query examples enabled
  - Profiler as query source
  - Custom collectors
  - 1GB max slowlog size

## Security Features

- **Authentication**: SCRAM-SHA-256
- **Users**: admin, backup, monitor, appuser, readonly (prod), analytics (prod)
- **TLS**: Disabled (dev), Preferred (standard), Required (production)
- **Encryption at Rest**: Production only
- **Network**: bindIpAll with security groups
- **Audit**: Client log data redaction (production)

## Resource Scaling

Operator otomatik olarak şunları yönetir:
- **Vertical Scaling**: Resource limit/request değişiklikleri
- **Horizontal Scaling**: Replica sayısı değişiklikleri
- **Storage Scaling**: PVC expansion (storage class destekliyorsa)
- **Rolling Updates**: Pod disruption budget ile güvenli

## Troubleshooting

```bash
# CR durumunu kontrol et
kubectl get psmdb -n <namespace>

# Operator logları
kubectl logs -n infraforge-operators deployment/percona-server-mongodb-operator

# MongoDB logları
kubectl logs -n <namespace> <pod-name> -c mongod

# Backup durumu
kubectl get psmdb-backup -n <namespace>
```