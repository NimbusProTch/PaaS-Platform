# Platform Templates

Generic template-based service catalog for InfraForge Platform.

## Structure

```
platform-templates/
├── operators/              # Kubernetes operators (cluster-scoped)
│   └── cloudnativepg/     # CloudNativePG PostgreSQL operator
│       ├── catalog.yaml   # Operator metadata
│       ├── Chart.yaml     # Helm chart (uses upstream)
│       └── values.yaml    # Operator configuration
│
├── services/              # Platform services (namespace-scoped)
│   └── postgresql/       # PostgreSQL database service
│       ├── catalog.yaml  # Service catalog definition
│       ├── nonprod/      # Non-production profile
│       │   ├── Chart.yaml
│       │   ├── values.yaml
│       │   └── templates/
│       │       ├── cluster.yaml      # Single instance
│       │       └── monitoring.yaml   # Basic monitoring
│       └── prod/         # Production profile
│           ├── Chart.yaml
│           ├── values.yaml
│           └── templates/
│               ├── cluster.yaml      # 3-instance HA cluster
│               ├── backup.yaml       # S3 backup
│               ├── pooler.yaml       # PgBouncer pooling
│               └── monitoring.yaml   # Full monitoring + alerts
│
└── business/             # Business application templates
    └── base/            # Base Helm chart for apps
        └── templates/
```

## Template Types

### 1. Operators
Kubernetes operators installed cluster-wide. Use upstream Helm charts with custom values.

**Example**: CloudNativePG operator for PostgreSQL management

### 2. Services
Platform services (databases, message queues, etc.) managed by operators.
Each service has multiple profiles (nonprod, prod).

**Example**: PostgreSQL cluster with nonprod (single instance) and prod (HA cluster with backup)

### 3. Business
User application templates (deployments, services, ingress).

## How It Works

1. **User creates a claim**:
   ```yaml
   apiVersion: platform.infraforge.io/v1
   kind: InfraForge
   spec:
     operators:
       - name: cloudnativepg
     platform:
       - name: postgresql
         profile: prod
   ```

2. **Generator processes templates**:
   - Loads catalog.yaml for metadata
   - Selects profile (nonprod/prod)
   - Merges values (catalog defaults + user params)
   - Renders Helm templates
   - Outputs manifests to Git

3. **ArgoCD deploys**:
   - Operators installed first
   - Platform services deployed
   - Business apps deployed last

## Adding New Services

To add a new service (e.g., RabbitMQ):

1. Create `services/rabbitmq/catalog.yaml`
2. Create `services/rabbitmq/nonprod/` templates
3. Create `services/rabbitmq/prod/` templates
4. No Go code changes needed!

## Benefits

✅ No hardcoded logic in Go generator
✅ Template-driven (easy to update)
✅ Profile-based (nonprod/prod)
✅ Production-ready configs built-in
✅ Catalog-based (self-documenting)
✅ Scalable (add services without code changes)
