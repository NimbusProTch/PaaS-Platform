# InfraForge Platform - Production-Ready Kubernetes Platform

## Overview
Enterprise-grade Kubernetes platform with GitOps, automated deployments, and multi-environment support.

## Features
- **GitOps with ArgoCD**: Automated deployments from Git
- **Multi-Environment**: Dev and Prod environments
- **Operators Included**:
  - PostgreSQL (CloudNativePG)
  - RabbitMQ
  - Redis
  - MinIO (Object Storage)
  - HashiCorp Vault (Secrets Management)

## Quick Start

### One-Command Installation
```bash
git clone https://github.com/NimbusProTch/PaaS-Platform.git
cd PaaS-Platform
git checkout feature/production-ready-platform
./setup.sh
```

This will:
1. Create a 4-node Kind cluster
2. Install all operators
3. Deploy ArgoCD
4. Configure environments (dev/prod)
5. Deploy all services

### Requirements
- Docker
- Kind
- kubectl
- Helm

## Architecture

### Environments
- **demo-dev**: Development environment with minimal resources
- **demo-prod**: Production environment with HA configurations

### Resource Allocations

#### Development Environment
| Service | CPU | Memory | Storage | Replicas |
|---------|-----|--------|---------|----------|
| PostgreSQL | 500m | 1Gi | 10Gi | 1 |
| RabbitMQ | 500m | 1Gi | 10Gi | 1 |
| Redis | 100m | 128Mi | - | 1 |
| MinIO | 500m | 1Gi | 10Gi | 1 |
| Vault | 100m | 256Mi | 5Gi | 1 |

#### Production Environment
| Service | CPU | Memory | Storage | Replicas |
|---------|-----|--------|---------|----------|
| PostgreSQL | 500m | 1Gi | 20Gi | 2 |
| RabbitMQ | 250m | 512Mi | 5Gi | 2 |
| Redis | 100m | 256Mi | 5Gi | 1 |
| MinIO | 250m | 512Mi | 10Gi | 2 |
| Vault | 100m | 256Mi | 5Gi | 1 |

## Access Services

### ArgoCD UI
```bash
kubectl port-forward svc/argocd-server -n infraforge-argocd 8080:443
# URL: https://localhost:8080
# Username: admin
# Password:
kubectl -n infraforge-argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

### PostgreSQL
```bash
# Dev environment
kubectl port-forward -n demo-dev svc/demo-postgresql-rw 5432:5432
# Credentials in secret: demo-postgresql-app-user

# Prod environment
kubectl port-forward -n demo-prod svc/demo-postgresql-rw 5433:5432
# Credentials in secret: demo-postgresql-app-user
```

### RabbitMQ Management
```bash
# Dev environment
kubectl port-forward -n demo-dev svc/demo-rabbitmq 15672:15672
# Default: guest/guest

# Prod environment
kubectl port-forward -n demo-prod svc/demo-rabbitmq 15673:15672
```

### MinIO Console
```bash
# Dev environment
kubectl port-forward -n demo-dev svc/minio 9001:9001
# Credentials in secret: demo-minio-env-configuration

# Prod environment
kubectl port-forward -n demo-prod svc/minio 9002:9001
```

### Vault UI
```bash
# Dev environment
kubectl port-forward -n demo-dev svc/demo-vault 8200:8300

# Prod environment
kubectl port-forward -n demo-prod svc/demo-vault 8201:8400
```

## Monitoring

### Check All Pods
```bash
kubectl get pods -A | grep demo-
```

### Check ArgoCD Applications
```bash
kubectl get applications -n infraforge-argocd
```

### Check Operator Status
```bash
# PostgreSQL clusters
kubectl get clusters.postgresql.cnpg.io -A

# RabbitMQ clusters
kubectl get rabbitmqclusters -A

# Redis clusters
kubectl get redis -A

# MinIO tenants
kubectl get tenants.minio.min.io -A
```

## Troubleshooting

### Pod Not Starting
```bash
kubectl describe pod <pod-name> -n <namespace>
kubectl logs <pod-name> -n <namespace>
```

### Force ArgoCD Sync
```bash
kubectl -n infraforge-argocd patch application <app-name> \
  --type merge -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}'
```

### Resource Issues
If pods are pending due to resources:
1. Check node capacity: `kubectl describe nodes`
2. Reduce resource requests in values.yaml files
3. Add more worker nodes to Kind cluster

## Clean Up

### Delete Everything
```bash
kind delete cluster --name infraforge-cluster
```

### Delete Specific Environment
```bash
kubectl delete namespace demo-dev  # or demo-prod
```

## Directory Structure
```
PaaS-Platform/
├── manifests/
│   └── platform-cluster/
│       ├── operators/
│       │   ├── dev/         # Dev environment configs
│       │   │   ├── postgresql/
│       │   │   ├── rabbitmq/
│       │   │   ├── redis/
│       │   │   ├── minio/
│       │   │   └── vault/
│       │   └── prod/        # Prod environment configs
│       │       ├── postgresql/
│       │       ├── rabbitmq/
│       │       ├── redis/
│       │       ├── minio/
│       │       └── vault/
│       └── appsets/         # ArgoCD ApplicationSets
│           ├── dev/
│           └── prod/
└── setup.sh                 # Installation script
```

## Key Features

### Auto-Generated Secrets
All passwords and credentials are automatically generated using Helm templates with random values.

### GitOps Workflow
All changes are tracked in Git and automatically deployed by ArgoCD.

### High Availability (Production)
Production environment configured with:
- Multiple replicas for databases
- Anti-affinity rules to spread across nodes
- Synchronous replication where applicable

### Resource Optimization
Resources are optimized for local development while maintaining production-like configurations.

## Contributing
1. Fork the repository
2. Create feature branch
3. Make changes
4. Test with `setup.sh`
5. Submit PR

## License
MIT