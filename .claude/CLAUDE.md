# Platform Architecture - Complete Documentation

> **Last Updated:** 2025-12-21
> **Status:** Active Development
> **Branch:** feature/custom-platform-operator

---

## 🎯 Platform Vision

**Single Source of Truth:** ApplicationClaim
**Zero Manual Deployment:** Her şey otomatik (sadece claim apply)
**Cloud Native:** Kubernetes Operators kullan (Bitnami değil!)
**GitOps Ready:** ArgoCD + ApplicationSet pattern
**Multi-Tenant:** Team ve environment bazlı izolasyon

---

## 📊 Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        DEVELOPER                             │
│              kubectl apply -f claim.yaml                     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                  PLATFORM OPERATOR                           │
│  1. Detect required operators (PostgreSQL, Redis, etc)      │
│  2. Auto-install operators (ArgoCD Application)              │
│  3. Create ApplicationSet (AppProject'e ata)                 │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                      ARGOCD                                  │
│  ApplicationSet → Generate Applications                      │
│  Fetch common chart from ChartMuseum                         │
│  Render based on type (microservice, postgresql, redis)     │
└────────────────────┬────────────────────────────────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
┌──────────────────┐    ┌──────────────────┐
│  MICROSERVICES   │    │  OPERATORS       │
│  Deployment      │    │  CloudNativePG   │
│  Service         │    │  Redis Operator  │
│  ConfigMap       │    │  RabbitMQ Op     │
└──────────────────┘    └────────┬─────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  CRDs Created   │
                        │  Cluster        │
                        │  RedisFailover  │
                        │  RabbitmqCluster│
                        └────────┬────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  FINAL PODS     │
                        │  PostgreSQL SS  │
                        │  Redis StatefulS│
                        │  RabbitMQ Pods  │
                        └─────────────────┘
```

---

## 🏗️ Component Responsibilities

### 1. **Terraform (Infrastructure Only)**

**Sorumluluklar:**
- ✅ VPC, Subnets, Security Groups
- ✅ EKS Cluster (Kubernetes)
- ✅ ECR Repositories (Docker images)
- ✅ ArgoCD (GitOps engine) - Helm ile
- ✅ ChartMuseum (Helm chart registry) - Helm ile
- ✅ Platform Operator (CRDs + Deployment) - kubectl ile
- ✅ EKS Addons (Metrics Server, Load Balancer Controller)
- ✅ ArgoCD AppProjects (team × environment matrix)

**Sorumlu OLMAYAN:**
- ❌ Uygulamalar (microservices)
- ❌ Veritabanları (PostgreSQL, Redis, etc)
- ❌ Operators (CloudNativePG, Redis Operator, etc)
- ❌ Monitoring stack (Prometheus, Grafana)

**Dosyalar:**
```
infrastructure/aws/
├── main.tf              # Provider, locals
├── vpc.tf               # VPC resources
├── eks.tf               # EKS cluster
├── ecr.tf               # ECR repositories
├── argocd.tf            # ArgoCD Helm release
├── argocd-projects.tf   # AppProjects (team × env)
├── chartmuseum.tf       # ChartMuseum Helm release
├── platform-operator.tf # Operator deployment + CRDs
├── addons.tf            # EKS addons
├── variables.tf         # Input variables
└── outputs.tf           # Outputs
```

---

### 2. **Platform Operator (Smart Controller)**

**Sorumluluklar:**
- ✅ ApplicationClaim CRD watch
- ✅ Gerekli operatörleri detect et
- ✅ Eksik operatörleri otomatik kur (ArgoCD Application via Helm)
- ✅ ApplicationSet oluştur (her claim için)
- ✅ Helm values generate et (type bazlı)
- ✅ AppProject assignment (team-environment)
- ✅ Lifecycle management (update, delete)

**Operator Logic:**

```go
func Reconcile(claim ApplicationClaim) {
    // 1. Required operators detect
    operators := detectRequiredOperators(claim)

    // 2. Eksik olanları kur
    for op in operators {
        if !exists(op) {
            installOperatorViaArgoCD(op)
        }
    }

    // 3. Wait for operators ready
    waitForOperators(operators)

    // 4. ApplicationSet oluştur
    createApplicationSet(claim)
}
```

**Type Detection:**

| Component Type | Required Operator | Helm Chart | Repo |
|----------------|------------------|------------|------|
| `postgresql` | CloudNativePG | cloudnative-pg | https://cloudnative-pg.github.io/charts |
| `redis` | Redis Operator | redis-operator | https://spotahome.github.io/redis-operator |
| `rabbitmq` | RabbitMQ Cluster Operator | cluster-operator | https://charts.bitnami.com/bitnami |
| `mongodb` | MongoDB Community Operator | community-operator | https://mongodb.github.io/helm-charts |
| `elasticsearch` | ECK Operator | eck-operator | https://helm.elastic.co |

**Dosyalar:**
```
infrastructure/platform-operator/
├── api/v1/
│   └── applicationclaim_types.go    # CRD definition
├── internal/controller/
│   ├── applicationclaim_controller.go  # Main reconciler
│   ├── argocd_controller.go            # ApplicationSet creation
│   ├── operator_installer.go           # Auto-install operators
│   ├── values_generator.go             # Helm values generation
│   └── utils.go                        # Helper functions
├── charts/common/                      # Template library
└── config/crd/                         # CRD manifests
```

---

### 3. **ArgoCD (GitOps Engine)**

**Sorumluluklar:**
- ✅ ApplicationSet expansion (list generator)
- ✅ Chart fetch (ChartMuseum)
- ✅ Helm template rendering (type-based)
- ✅ Kubernetes resource sync
- ✅ Health monitoring
- ✅ Auto-sync / self-heal

**ApplicationSet Pattern:**

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: ecommerce-qa-appset
  namespace: argocd
  labels:
    platform.infraforge.io/claim: ecommerce-qa
    platform.infraforge.io/team: ecommerce-team
    platform.infraforge.io/environment: qa
spec:
  generators:
    - list:
        elements:
          # Microservices
          - name: product-service
            type: microservice
            image: "...ecr.../product-service:latest"
            replicas: "2"

          # Platform components
          - name: main-db
            type: postgresql
            version: "16"
            replicas: "3"
            storage: "50Gi"

          - name: cache
            type: redis
            mode: cluster
            replicas: "6"

  template:
    metadata:
      name: 'ecommerce-qa-{{name}}'
    spec:
      project: ecommerce-team-qa  # ← AppProject
      source:
        repoURL: http://chartmuseum.chartmuseum.svc:8080
        chart: common
        targetRevision: 2.0.0
        helm:
          valuesObject:
            type: '{{type}}'              # ← Conditional rendering
            fullnameOverride: '{{name}}'
            replicaCount: '{{replicas}}'
            # ... dynamic values
      destination:
        server: https://kubernetes.default.svc
        namespace: qa
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

---

### 4. **ChartMuseum (Helm Chart Registry)**

**Sorumluluklar:**
- ✅ Helm chart storage (common chart)
- ✅ Chart versioning
- ✅ HTTP API (push/pull)

**Chart Structure:**

```
charts/common/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── _helpers.tpl
    │
    ├── microservice/
    │   ├── deployment.yaml      # type=microservice
    │   ├── service.yaml
    │   ├── configmap.yaml
    │   └── hpa.yaml
    │
    └── platform/
        ├── postgresql-cluster.yaml    # type=postgresql (CloudNativePG CRD)
        ├── redis-failover.yaml        # type=redis (Redis Operator CRD)
        ├── rabbitmq-cluster.yaml      # type=rabbitmq (RabbitMQ Operator CRD)
        ├── mongodb-replicaset.yaml    # type=mongodb (MongoDB Operator CRD)
        └── elasticsearch.yaml         # type=elasticsearch (ECK CRD)
```

**Conditional Rendering:**

```yaml
# templates/platform/postgresql-cluster.yaml
{{- if eq .Values.type "postgresql" }}
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: {{ .Values.fullnameOverride }}
spec:
  instances: {{ .Values.replicaCount | default 3 }}
  postgresql:
    parameters:
      max_connections: {{ .Values.config.maxConnections | default "200" }}
  storage:
    size: {{ .Values.storage | default "20Gi" }}
    storageClass: {{ .Values.storageClass | default "gp3" }}
  backup:
    {{- if .Values.config.backup.enabled }}
    barmanObjectStore:
      destinationPath: s3://{{ .Values.config.backup.s3Bucket }}/{{ .Values.fullnameOverride }}
      s3Credentials:
        inheritFromIAMRole: true
    {{- end }}
{{- end }}
```

---

## 📋 ApplicationClaim Structure

### **Complete Example:**

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: ecommerce-prod
  namespace: default
spec:
  # Target namespace
  namespace: prod

  # Environment (dev, qa, staging, prod)
  environment: prod

  # Ownership
  owner:
    team: Ecommerce Team
    email: ecommerce@company.com

  # Microservices
  applications:
    - name: product-service
      image: 715841344657.dkr.ecr.eu-west-1.amazonaws.com/infraforge-prod/product-service:v2.0.0
      replicas: 5
      ports:
        - name: http
          port: 8080
      env:
        - name: DATABASE_URL
          value: postgresql://main-db:5432/products
      resources:
        cpu: 1000m
        memory: 2Gi
      autoscaling:
        enabled: true
        minReplicas: 5
        maxReplicas: 20
        targetCPU: 70

    - name: user-service
      image: 715841344657.dkr.ecr.eu-west-1.amazonaws.com/infraforge-prod/user-service:v2.0.0
      replicas: 3
      ports:
        - port: 8081

  # Platform Components
  components:
    # PostgreSQL (CloudNativePG Operator)
    - type: postgresql
      name: main-db
      version: "16"
      config:
        replicas: 3
        storage: 200Gi
        storageClass: gp3-retain
        maxConnections: 500
        sharedBuffers: 4GB
        backup:
          enabled: true
          s3Bucket: prod-backups
          schedule: "0 2 * * *"
          retention: 30d

    # Redis (Redis Operator)
    - type: redis
      name: cache
      version: "7.2"
      config:
        mode: cluster
        replicas: 6
        storage: 50Gi
        resources:
          cpu: 500m
          memory: 2Gi

    # RabbitMQ (RabbitMQ Cluster Operator)
    - type: rabbitmq
      name: queue
      version: "3.12"
      config:
        replicas: 3
        storage: 30Gi
        resources:
          cpu: 1000m
          memory: 4Gi

    # MongoDB (MongoDB Community Operator)
    - type: mongodb
      name: analytics-db
      version: "7.0"
      config:
        type: ReplicaSet
        members: 3
        storage: 100Gi

    # Elasticsearch (ECK Operator)
    - type: elasticsearch
      name: search
      version: "8.11"
      config:
        nodes:
          master: 3
          data: 5
          ingest: 2
        storage: 500Gi
        resources:
          cpu: 4000m
          memory: 16Gi
```

### **Field Reference:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.namespace` | string | ✅ | Target K8s namespace |
| `spec.environment` | string | ✅ | dev, qa, staging, prod |
| `spec.owner.team` | string | ✅ | Team name (for AppProject) |
| `spec.owner.email` | string | ✅ | Contact email |
| `spec.applications[]` | array | ❌ | Microservices list |
| `spec.applications[].name` | string | ✅ | App name |
| `spec.applications[].image` | string | ✅ | Docker image |
| `spec.applications[].replicas` | int | ✅ | Pod count |
| `spec.components[]` | array | ❌ | Platform components |
| `spec.components[].type` | string | ✅ | postgresql, redis, rabbitmq, etc |
| `spec.components[].name` | string | ✅ | Instance name |
| `spec.components[].version` | string | ❌ | Component version |
| `spec.components[].config` | map | ❌ | Type-specific config |

---

## 🔄 Complete Deployment Flow

### **Step 1: Infrastructure Setup (Once)**

```bash
cd infrastructure/aws
terraform init
terraform apply -var environment=prod
```

**Result:**
```
✅ VPC created
✅ EKS cluster running (infraforge-prod)
✅ ECR repositories created
✅ ArgoCD installed (https://argocd-prod.domain.com)
✅ ChartMuseum installed (http://chartmuseum.chartmuseum.svc:8080)
✅ Platform Operator deployed (2/2 pods running)
✅ AppProjects created (ecommerce-team-prod, analytics-team-prod, etc)
```

---

### **Step 2: Push Code (Continuous)**

```bash
git push origin main
```

**GitHub Actions Triggered:**

1. **Build Microservices** (`.github/workflows/build-microservices.yml`)
   - Detect changes in `microservices/**`
   - Build Docker images
   - Push to ECR with tags: `latest`, `<commit-sha>`, `v1.0.0`

2. **Build Operator** (`.github/workflows/build-operator.yml`)
   - Detect changes in `infrastructure/platform-operator/**`
   - Run Go tests
   - Build operator image
   - Push to ECR: `platform-operator:v2.X.0`

3. **Build Charts** (`.github/workflows/build-charts.yml`)
   - Detect changes in `charts/**`
   - Lint charts
   - Package charts
   - Push to ChartMuseum

---

### **Step 3: Deploy Application (Developer)**

```bash
kubectl apply -f ecommerce-prod-claim.yaml
```

**Operator Logs:**

```
[00:00] Reconciling ApplicationClaim: ecommerce-prod
[00:01] Detecting required operators...
[00:02]   - postgresql → CloudNativePG
[00:02]   - redis → Redis Operator
[00:02]   - rabbitmq → RabbitMQ Cluster Operator
[00:03] Checking CloudNativePG operator...
[00:03]   ❌ Not found, installing via ArgoCD...
[00:05]   ✅ ArgoCD Application created: cloudnative-pg
[00:06] Checking Redis Operator...
[00:06]   ❌ Not found, installing...
[00:08]   ✅ ArgoCD Application created: redis-operator
[00:10] Checking RabbitMQ Operator...
[00:10]   ✅ Already installed
[00:15] Waiting for operators to be ready...
[01:30] ✅ All operators ready!
[01:31] Creating ApplicationSet: ecommerce-prod-prod-appset
[01:32]   - Project: ecommerce-team-prod
[01:32]   - Elements: 7 (2 microservices, 5 components)
[01:33] ✅ ApplicationSet created successfully!
```

**ArgoCD ApplicationSet:**

```bash
kubectl get applicationset -n argocd
```
```
NAME                        AGE
ecommerce-prod-prod-appset  2m
```

**ArgoCD Applications Generated:**

```bash
kubectl get application -n argocd
```
```
NAME                              SYNC STATUS   HEALTH
cloudnative-pg                    Synced        Healthy
redis-operator                    Synced        Healthy
ecommerce-prod-product-service    Synced        Healthy
ecommerce-prod-user-service       Synced        Healthy
ecommerce-prod-main-db            Synced        Healthy
ecommerce-prod-cache              Synced        Healthy
ecommerce-prod-queue              Synced        Healthy
```

**Final Resources:**

```bash
kubectl get all -n prod
```
```
NAME                                    READY   STATUS
pod/product-service-xxx                 1/1     Running
pod/product-service-yyy                 1/1     Running
pod/user-service-xxx                    1/1     Running
pod/main-db-1                           1/1     Running
pod/main-db-2                           1/1     Running
pod/main-db-3                           1/1     Running
pod/cache-0                             1/1     Running
pod/cache-1                             1/1     Running
pod/queue-server-0                      1/1     Running
```

---

## 🏢 Multi-Environment Strategy

### **AppProjects (Team × Environment Matrix)**

**Terraform oluşturur:**

```hcl
teams = ["ecommerce-team", "analytics-team", "crm-team"]
environments = ["dev", "qa", "staging", "prod"]

# 3 teams × 4 envs = 12 AppProjects
for_each = setproduct(teams, environments)

AppProject: ecommerce-team-dev
AppProject: ecommerce-team-qa
AppProject: ecommerce-team-staging
AppProject: ecommerce-team-prod
AppProject: analytics-team-dev
...
```

**Operator assigns:**

```go
teamSlug := sanitize(claim.Spec.Owner.Team)  // "Ecommerce Team" → "ecommerce-team"
projectName := teamSlug + "-" + claim.Spec.Environment  // "ecommerce-team-prod"

applicationSet.Spec.Template.Spec.Project = projectName
```

**ArgoCD UI:**

```
Projects:
├─ ecommerce-team-prod (5 apps)
│  ├─ ecommerce-prod-product-service
│  ├─ ecommerce-prod-user-service
│  ├─ ecommerce-prod-main-db
│  ├─ ecommerce-prod-cache
│  └─ ecommerce-prod-queue
│
└─ analytics-team-qa (3 apps)
   ├─ analytics-qa-event-processor
   ├─ analytics-qa-clickhouse
   └─ analytics-qa-kafka
```

**RBAC:**

```yaml
# AppProject spec.roles
roles:
  - name: developer
    policies:
      - p, proj:ecommerce-team-prod:developer, applications, get, ecommerce-team-prod/*, allow
      - p, proj:ecommerce-team-prod:developer, applications, sync, ecommerce-team-prod/*, allow
    groups:
      - ecommerce-team-developers

  - name: admin
    policies:
      - p, proj:ecommerce-team-prod:admin, applications, *, ecommerce-team-prod/*, allow
    groups:
      - ecommerce-team-admins
      - platform-admins
```

---

## 🎯 Production-Ready Operators

### **PostgreSQL: CloudNativePG**

**Why?**
- ✅ CNCF Sandbox project
- ✅ Streaming replication (automatic failover)
- ✅ Point-in-time recovery (PITR)
- ✅ Integrated connection pooling (PgBouncer)
- ✅ Declarative backups (S3, GCS, Azure)
- ✅ Rolling updates (zero downtime)
- ✅ Monitoring (Prometheus ServiceMonitor)

**CRD Example:**

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: main-db
spec:
  instances: 3
  postgresql:
    parameters:
      max_connections: "500"
      shared_buffers: "4GB"
  storage:
    size: 200Gi
    storageClass: gp3-retain
  backup:
    barmanObjectStore:
      destinationPath: s3://prod-backups/main-db
      s3Credentials:
        inheritFromIAMRole: true
      wal:
        compression: gzip
    retentionPolicy: "30d"
  monitoring:
    enablePodMonitor: true
```

### **Redis: Redis Operator (Spotahome)**

**Why?**
- ✅ Sentinel mode (automatic failover)
- ✅ Cluster mode (sharding)
- ✅ Backup/restore support
- ✅ Redis 7.x support
- ✅ Custom configuration

**CRD Example:**

```yaml
apiVersion: databases.spotahome.com/v1
kind: RedisFailover
metadata:
  name: cache
spec:
  sentinel:
    replicas: 3
  redis:
    replicas: 6
    storage:
      persistentVolumeClaim:
        metadata:
          name: cache-data
        spec:
          accessModes: [ReadWriteOnce]
          resources:
            requests:
              storage: 50Gi
```

### **RabbitMQ: Cluster Operator (Official)**

**Why?**
- ✅ Official VMware operator
- ✅ Cluster formation automatic
- ✅ Quorum queues
- ✅ Plugin management
- ✅ TLS support

**CRD Example:**

```yaml
apiVersion: rabbitmq.com/v1beta1
kind: RabbitmqCluster
metadata:
  name: queue
spec:
  replicas: 3
  resources:
    requests:
      cpu: 1000m
      memory: 4Gi
  persistence:
    storage: 30Gi
    storageClassName: gp3
```

---

## 🔧 Development Workflow

### **Local Development (Orbstack)**

1. **Start local Kubernetes:**
   ```bash
   # Orbstack automatically provides K8s cluster
   kubectl config use-context orbstack
   ```

2. **Deploy minimal infrastructure:**
   ```bash
   # ArgoCD
   kubectl create namespace argocd
   kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

   # ChartMuseum
   helm repo add chartmuseum https://chartmuseum.github.io/charts
   helm install chartmuseum chartmuseum/chartmuseum -n chartmuseum --create-namespace

   # Platform Operator
   cd infrastructure/platform-operator
   make install  # Install CRDs
   make run      # Run locally
   ```

3. **Test ApplicationClaim:**
   ```bash
   kubectl apply -f ecommerce-claim.yaml
   kubectl logs -n platform-operator-system -l control-plane=controller-manager -f
   ```

### **Git Workflow**

**Branch Strategy:**
- `main` - Production ready
- `develop` - Integration
- `feature/*` - Feature branches

**Commit Convention:**
```bash
feat: Add PostgreSQL operator auto-install
fix: Fix ApplicationSet project assignment
chore: Update dependencies
docs: Update CLAUDE.md
```

**Every Change:**
```bash
# 1. Make changes
# 2. Test locally
# 3. Commit
git add .
git commit -m "feat: Description"

# 4. Push
git push origin feature/custom-platform-operator
```

---

## 📦 Repository Structure

```
PaaS-Platform/
├── .claude/
│   ├── CLAUDE.md          # This file (architecture doc)
│   ├── rules.md           # Strict development rules
│   └── workflow.md        # Development workflow
│
├── .github/workflows/
│   ├── build-microservices.yml
│   ├── build-operator.yml
│   └── build-charts.yml
│
├── infrastructure/
│   ├── aws/
│   │   ├── main.tf
│   │   ├── vpc.tf
│   │   ├── eks.tf
│   │   ├── ecr.tf
│   │   ├── argocd.tf
│   │   ├── argocd-projects.tf
│   │   ├── chartmuseum.tf
│   │   ├── platform-operator.tf
│   │   └── addons.tf
│   │
│   └── platform-operator/
│       ├── api/v1/
│       ├── internal/controller/
│       ├── charts/common/
│       ├── config/
│       ├── Makefile
│       └── Dockerfile.simple
│
├── microservices/
│   ├── product-service/
│   ├── user-service/
│   ├── order-service/
│   ├── payment-service/
│   └── notification-service/
│
└── claims/
    ├── ecommerce-qa-claim.yaml
    ├── ecommerce-prod-claim.yaml
    └── analytics-prod-claim.yaml
```

---

## ✅ Success Criteria

**Infrastructure:**
- [ ] Terraform apply başarılı (EKS + ArgoCD + ChartMuseum + Operator)
- [ ] AppProjects oluşturulmuş (team × environment matrix)
- [ ] Operator çalışıyor (2/2 pods)

**Operator:**
- [ ] ApplicationClaim apply edilince reconcile çalışıyor
- [ ] Required operators detect ediliyor
- [ ] Operators otomatik kuruluyor (ArgoCD Application)
- [ ] ApplicationSet oluşturuluyor
- [ ] AppProject doğru assign ediliyor

**ArgoCD:**
- [ ] ApplicationSet expand oluyor (list generator)
- [ ] Applications oluşuyor (her element için)
- [ ] Chart fetch ediliyor (ChartMuseum)
- [ ] Helm render çalışıyor (type-based)
- [ ] Resources sync oluyor

**Final:**
- [ ] Microservices deploy oluyor (Deployment + Service)
- [ ] PostgreSQL cluster oluşuyor (CloudNativePG)
- [ ] Redis cluster oluşuyor (Redis Operator)
- [ ] RabbitMQ cluster oluşuyor (RabbitMQ Operator)
- [ ] All pods healthy

---

## 🚀 Next Steps

### **Phase 1: Chart Development** (Current)
- [ ] Create `charts/common/` structure
- [ ] Add microservice templates
- [ ] Add platform component templates (PostgreSQL, Redis, RabbitMQ)
- [ ] Test conditional rendering
- [ ] Package and push to ChartMuseum

### **Phase 2: Operator Enhancement**
- [ ] Add `type` field to ApplicationClaim CRD
- [ ] Implement operator auto-install logic
- [ ] Update ApplicationSet creation (project assignment)
- [ ] Add rich labels/annotations
- [ ] Test with multiple environments

### **Phase 3: Terraform Updates**
- [ ] Add `argocd-projects.tf` (team × env matrix)
- [ ] Add `chartmuseum.tf`
- [ ] Update `platform-operator.tf` (latest manifests)
- [ ] Test infrastructure deployment

### **Phase 4: GitHub Actions**
- [ ] Create chart build/push workflow
- [ ] Update microservices workflow (ECR tags)
- [ ] Update operator workflow (version bump)
- [ ] Test CI/CD pipeline

### **Phase 5: Integration Testing**
- [ ] Deploy to Orbstack local cluster
- [ ] Test single environment claim
- [ ] Test multi-environment claims
- [ ] Test operator updates
- [ ] Test component scaling

### **Phase 6: Production Readiness**
- [ ] Add monitoring (Prometheus + Grafana)
- [ ] Add logging (Loki)
- [ ] Add tracing (Tempo)
- [ ] Add backup/restore procedures
- [ ] Security hardening
- [ ] Documentation completion

---

## 📞 Contact & Support

**Team:** Platform Engineering
**Owner:** Ecommerce Team
**Email:** platform@company.com
**Slack:** #platform-support

**Documentation:**
- Architecture: `.claude/CLAUDE.md` (this file)
- Rules: `.claude/rules.md`
- Workflow: `.claude/workflow.md`

**Repository:** https://github.com/NimbusProTch/PaaS-Platform

---

> **Last Updated:** 2025-12-21
> **Version:** 2.0.0
> **Status:** 🟡 Active Development
