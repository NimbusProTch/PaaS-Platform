# Application Claims - Deployments

This directory contains ApplicationClaim manifests organized by environment.

## Directory Structure

```
deployments/
├── dev/                                      # Development environment
│   ├── platform-infrastructure-claim.yaml   # Infrastructure services (PostgreSQL, Redis, etc.)
│   └── apps-claim.yaml                      # Application microservices
├── staging/                                  # Staging environment
│   └── README.md
└── prod/                                     # Production environment
    └── README.md
```

## Two-Claim Architecture

The platform is split into two ApplicationClaims for better modularity:

### 1. Platform Infrastructure (`platform-infrastructure-claim.yaml`)
**Purpose**: Foundational infrastructure services that applications depend on

**Components (8 total)**:
- PostgreSQL × 5 (product-db, user-db, order-db, payment-db, notification-db)
- Redis × 1 (shared cache)
- RabbitMQ × 1 (message broker)
- Elasticsearch × 1 (product search)

**Why separate?**:
- ✅ Infrastructure is stable, rarely changes
- ✅ Can be managed by platform/infra team
- ✅ Deploy once, use by all apps
- ✅ Smaller YAML files (~120 lines)

### 2. Applications (`apps-claim.yaml`)
**Purpose**: Business logic microservices

**Services (5 total)**:
- product-service (Node.js, Port 8080)
- user-service (Go, Port 8081)
- order-service (Go, Port 8082)
- payment-service (Node.js, Port 8083)
- notification-service (Go, Port 8084)

**Why separate?**:
- ✅ Apps change frequently (code updates, scaling, config)
- ✅ Can be managed by app teams
- ✅ Independent deployments
- ✅ Smaller YAML files (~240 lines vs 460 lines combined)

## ApplicationClaim Overview

The ecommerce platform consists of:

### Microservices (5 total)
1. **product-service** (Node.js) - Port 8080
   - Product catalog management
   - Elasticsearch integration for search
   - Prometheus metrics on port 9090

2. **user-service** (Go) - Port 8081
   - User authentication and management
   - Redis session storage

3. **order-service** (Go) - Port 8082
   - Order processing and management
   - Event-driven with RabbitMQ

4. **payment-service** (Node.js) - Port 8083
   - Payment processing with Stripe
   - Prometheus metrics on port 9090

5. **notification-service** (Go) - Port 8084
   - Email/SMS notifications
   - SMTP and Twilio integration

### Infrastructure Components (8 total)
1. **PostgreSQL** (5 instances)
   - Separate database per microservice
   - CloudNativePG operator-managed

2. **Redis** (1 instance)
   - Shared cache and session store
   - Redis Failover for HA

3. **RabbitMQ** (1 instance)
   - Message broker for async communication
   - RabbitMQ Cluster Operator

4. **Elasticsearch** (1 instance)
   - Product search indexing
   - ECK operator-managed

## Environment-Specific Configuration

### Development (dev/)
- Lower resource limits
- Debug logging enabled
- Single replicas for most services
- Development credentials

### Staging (staging/)
- Production-like configuration
- Higher resource allocation
- Multiple replicas for testing HA
- Staging credentials

### Production (prod/)
- Production-grade resources
- High availability (3+ replicas)
- Monitoring and alerting enabled
- Production credentials (from secrets)

## Deployment

### Deploy to Development

**Step 1: Deploy Infrastructure** (required first)
```bash
kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
```

**Step 2: Wait for Infrastructure Ready** (~2-5 minutes)
```bash
kubectl get applicationclaim ecommerce-infrastructure -w
kubectl get pods -l platform.infraforge.io/claim=ecommerce-infrastructure
```

**Step 3: Deploy Applications**
```bash
kubectl apply -f deployments/dev/apps-claim.yaml
```

**One-liner** (deploy both with sync waves):
```bash
# With sync waves enabled, ArgoCD automatically waits for infrastructure
# No manual sleep needed - just apply both!
kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
kubectl apply -f deployments/dev/apps-claim.yaml
```

**Note**: ArgoCD Sync Waves ensure infrastructure (wave 0) deploys before apps (wave 1)

### Watch Status
```bash
kubectl get applicationclaim ecommerce-platform -w
kubectl get applicationset -n argocd
kubectl get application -n argocd
```

### Verify Deployments
```bash
# Check pods
kubectl get pods -l platform.infraforge.io/claim=ecommerce-platform

# Check services
kubectl get svc -l platform.infraforge.io/claim=ecommerce-platform

# Check PostgreSQL clusters
kubectl get cluster
```

## Resource Requirements

### Total Resources (Development)
- **CPU Requests**: ~2.75 cores
- **CPU Limits**: ~5.5 cores
- **Memory Requests**: ~6.5 GB
- **Memory Limits**: ~13 GB
- **Storage**: ~120 GB

### EKS Node Recommendations
- **Development**: 2x t3.large (2 vCPU, 8 GB RAM each)
- **Staging**: 3x t3.xlarge (4 vCPU, 16 GB RAM each)
- **Production**: 5x t3.2xlarge (8 vCPU, 32 GB RAM each)

## Dependencies

All microservices depend on:
- PostgreSQL (dedicated instance per service)
- Redis (shared)
- RabbitMQ (shared)

Product service additionally depends on:
- Elasticsearch

## Configuration

### Image Registry Override

By default, the operator pulls images from `ghcr.io/nimbusprotch`. To use a different registry:

**Option 1: Environment Variable** (Recommended)
```yaml
# infrastructure/platform-operator/config/manager/deployment.yaml
env:
  - name: IMAGE_REGISTRY
    value: "ghcr.io/YOUR-ORG"  # Your GitHub Container Registry org
    # OR
    value: "715841344657.dkr.ecr.eu-west-1.amazonaws.com/YOUR-REPO"  # AWS ECR
    # OR
    value: "docker.io/YOUR-ORG"  # Docker Hub
```

**Option 2: Per-Application Override**
```yaml
# deployments/dev/apps-claim.yaml
applications:
  - name: product-service
    image: ghcr.io/CUSTOM-ORG/product-service  # Full image URL
    version: v1.0.0
```

## Next Steps

1. Deploy infrastructure: `cd infrastructure/aws && tofu apply`
2. Deploy operator: `kubectl apply -f infrastructure/platform-operator/config/`
3. Deploy claims:
   ```bash
   kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
   kubectl apply -f deployments/dev/apps-claim.yaml
   ```
4. Wait for ArgoCD sync: `kubectl get app -n argocd -w`
5. Verify all pods running: `kubectl get pods`
