# 🚨 STRICT DEVELOPMENT RULES - PLATFORM OPERATOR

> **UYARI:** Bu kurallar MUT

LAK takip edilmeli. Hiçbir istisna yok!

---

## 📜 CORE PRINCIPLES

### 1. **HER ZAMAN TÜRKÇE CEVAP VER**

```
❌ YANLIŞ:
"I'll create the ApplicationSet now..."

✅ DOĞRU:
"ApplicationSet'i şimdi oluşturuyorum..."
```

**İstisna:** Kod, YAML, commit message İngilizce olabilir.

---

### 2. **BELİRLENEN YAPININ DIŞINA ASLA ÇIKMA**

**Agreed Architecture (CLAUDE.md'de dokümante):**

```
ApplicationClaim
    ↓
Platform Operator (auto-install operators)
    ↓
ApplicationSet (AppProject assigned)
    ↓
ArgoCD (chart fetch + render)
    ↓
Kubernetes Resources
```

**YAPILMAYACAKLAR:**

❌ Terraform'da uygulama/operator deployment
❌ Bitnami chart kullanımı (sadece production-ready operators)
❌ Manuel kubectl apply (sadece claim hariç)
❌ ArgoCD Application (sadece ApplicationSet)
❌ Hardcoded values (her şey claim'den)
❌ Git-based GitOps (K8s-native, ChartMuseum)

**YAPILACAKLAR:**

✅ Terraform: Sadece infrastructure (EKS, ArgoCD, ChartMuseum, Operator, AppProjects)
✅ Operator: Smart auto-install + ApplicationSet creation
✅ ChartMuseum: Common chart (type-based templates)
✅ ApplicationClaim: Single source of truth

---

### 3. **HİÇBİR ŞEYİ MANUEL DEPLOY ETME**

**Allowed:**
```bash
# SADECE BU İZİNLİ:
kubectl apply -f ecommerce-claim.yaml
```

**NOT Allowed:**
```bash
# BUNLAR YASAK:
kubectl apply -f postgres-deployment.yaml  ❌
kubectl apply -f redis-statefulset.yaml    ❌
helm install postgresql bitnami/postgresql ❌
terraform apply (operators için)           ❌
```

**Her şey otomatik olmalı:**
- Operator, claim'i görünce operators'ları kurar
- Operator, ApplicationSet oluşturur
- ArgoCD, ApplicationSet'ten applications generate eder
- ArgoCD, resources'ları sync eder

---

### 4. **HER DEĞİŞİKLİKTEN SONRA COMMIT + PUSH**

**Workflow:**

```bash
# 1. Değişiklik yap
vim infrastructure/platform-operator/internal/controller/argocd_controller.go

# 2. Test et (local)
make run

# 3. MUTLAKA commit et
git add .
git commit -m "feat: Add operator auto-install logic"

# 4. MUTLAKA push et
git push origin feature/custom-platform-operator
```

**Commit Convention:**

```
feat:     Yeni özellik
fix:      Bug fix
chore:    Dependency update, cleanup
docs:     Documentation update
refactor: Code refactoring
test:     Test ekleme
```

**UNUTMA:** Her değişiklik mutlaka Git'e kaydedilmeli!

---

### 5. **ORBSTACK KULLAN (Local Development)**

**Setup:**

```bash
# 1. Orbstack K8s cluster kullan
kubectl config use-context orbstack

# 2. Local deploy
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# 3. ChartMuseum
helm install chartmuseum chartmuseum/chartmuseum -n chartmuseum --create-namespace

# 4. Operator (local run)
cd infrastructure/platform-operator
make install  # CRDs
make run      # Controller
```

**YASAK:**

❌ Production cluster'da test yapma
❌ Manuel deployment
❌ Orbstack dışında local cluster (minikube, kind, etc) kullanma

---

### 6. **BİR ŞEY ÇALIŞMIYORSA SÖYLEBİLİRSİN, DEĞİŞTİRME!**

**Doğru Yaklaşım:**

```
Senaryo: PostgreSQL operator kurulumu fail ediyor

❌ YANLIŞ:
"Let me try using Bitnami chart instead..."

✅ DOĞRU:
"CloudNativePG operator kurulumu şu hatayla fail etti:
[error log]

Sorun şu olabilir:
1. CRD versiyonu uyumsuz
2. RBAC permissions eksik
3. Helm repo erişim sorunu

Nasıl düzeltmemi istersin?"
```

**Prensip:**
- ✅ Sorun tespit et
- ✅ Olası çözümleri öner
- ✅ User'dan onay al
- ❌ Agreed yapıyı bozmadan çöz

---

### 7. **ULTRA THINK - HER ADIMI DÜŞÜN**

**Thinking Process:**

```
1. İSTENEN: ApplicationClaim'den PostgreSQL oluştur
2. MİMARİ: Operator → ApplicationSet → ArgoCD → CloudNativePG CRD
3. DEPENDENCIES:
   - CloudNativePG operator kurulu mu?
   - Common chart PostgreSQL template'i var mı?
   - ChartMuseum erişilebilir mi?
4. ADIMLAR:
   a. Operator: CloudNativePG operator'u kontrol et
   b. Yoksa: ArgoCD Application ile kur
   c. Bekle: Operator ready olana kadar
   d. ApplicationSet oluştur: type=postgresql element ekle
   e. ArgoCD: Chart render et → CRD oluştur
5. VALIDATION:
   - kubectl get cluster -n <namespace>
   - kubectl get pods -n <namespace>
   - kubectl logs <pod-name>
```

**Her değişiklik öncesi:**
- ❓ Bu agreed architecture'a uygun mu?
- ❓ Manual step var mı? (olmamalı!)
- ❓ Claim dışında configuration var mı? (olmamalı!)
- ❓ Test edilebilir mi? (Orbstack'te)

---

## 🎯 COMPONENT-SPECIFIC RULES

### **Terraform**

**ALLOWED:**
```hcl
✅ VPC, Subnets, Security Groups
✅ EKS Cluster
✅ ECR Repositories
✅ ArgoCD (Helm release)
✅ ChartMuseum (Helm release)
✅ Platform Operator (kubectl manifest)
✅ AppProjects (kubectl manifest)
✅ EKS Addons (Metrics Server, ALB Controller)
```

**NOT ALLOWED:**
```hcl
❌ Applications (microservices)
❌ Databases (PostgreSQL, Redis, etc)
❌ Operators (CloudNativePG, Redis Operator, etc)
❌ Monitoring stack (Prometheus, Grafana)
❌ Hardcoded application configs
```

---

### **Platform Operator**

**RESPONSIBILITIES:**
```go
✅ ApplicationClaim CRD watch
✅ Detect required operators (from claim.spec.components[].type)
✅ Auto-install operators (ArgoCD Application via Helm)
✅ Wait for operators ready
✅ Create ApplicationSet (with AppProject assignment)
✅ Generate Helm values (type-based)
✅ Lifecycle management (update, delete)
```

**FORBIDDEN:**
```go
❌ Direct kubectl apply
❌ Helm install directly
❌ Hardcoded operator versions (should be configurable)
❌ Bitnami chart references
❌ Git operations (no GitOps repo push)
```

---

### **ApplicationClaim**

**VALID:**
```yaml
✅ spec.namespace: qa
✅ spec.environment: prod
✅ spec.owner.team: Ecommerce Team
✅ spec.applications[]: microservices
✅ spec.components[]: postgresql, redis, rabbitmq, mongodb, elasticsearch
✅ spec.components[].config: type-specific config
```

**INVALID:**
```yaml
❌ Hardcoded image tags (should be latest or version from claim)
❌ Hardcoded replicas (should be from claim)
❌ Hardcoded resources (should be from claim or environment-based)
❌ External URLs (should be service names)
❌ Secrets in plaintext (should be references)
```

---

### **Charts (ChartMuseum)**

**STRUCTURE:**
```
charts/common/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── _helpers.tpl
    ├── microservice/      # type=microservice
    └── platform/          # type=postgresql, redis, etc
```

**RULES:**
```yaml
✅ Single common chart (not multiple charts)
✅ Conditional rendering ({{- if eq .Values.type "postgresql" }})
✅ All values from ApplicationClaim
✅ No hardcoded values
✅ CRDs for operators (not StatefulSets)
```

**EXAMPLE:**
```yaml
# templates/platform/postgresql-cluster.yaml
{{- if eq .Values.type "postgresql" }}
apiVersion: postgresql.cnpg.io/v1  # ✅ CloudNativePG CRD
kind: Cluster
metadata:
  name: {{ .Values.fullnameOverride }}
spec:
  instances: {{ .Values.replicaCount }}
  storage:
    size: {{ .Values.storage }}
{{- end }}

# ❌ NOT StatefulSet:
# apiVersion: apps/v1
# kind: StatefulSet
```

---

## 🔍 VALIDATION CHECKLIST

**Her değişiklik sonrası:**

### **Code Quality:**
- [ ] Go kod linted mi? (`make lint`)
- [ ] Tests pass mi? (`make test`)
- [ ] YAML valid mi? (`yamllint`)
- [ ] Terraform plan çalışıyor mu? (`terraform plan`)

### **Architecture Compliance:**
- [ ] Agreed architecture'a uygun mu?
- [ ] Manuel step yok mu?
- [ ] Hardcoded value yok mu?
- [ ] Production-ready operator kullanılmış mı? (Bitnami değil)

### **Git:**
- [ ] Commit message convention'a uygun mu?
- [ ] Branch doğru mu? (`feature/custom-platform-operator`)
- [ ] Push yapıldı mı?

### **Testing:**
- [ ] Orbstack'te test edildi mi?
- [ ] ApplicationClaim apply edilebildi mi?
- [ ] ApplicationSet oluştu mu?
- [ ] Resources deploy oldu mu?

---

## 🚫 COMMON MISTAKES TO AVOID

### **1. Bitnami Charts**

```yaml
# ❌ YANLIŞ:
source:
  repoURL: https://charts.bitnami.com/bitnami
  chart: postgresql

# ✅ DOĞRU:
source:
  repoURL: http://chartmuseum.chartmuseum.svc:8080
  chart: common
  helm:
    valuesObject:
      type: postgresql  # → CloudNativePG CRD render edilir
```

### **2. Terraform'da Application Deploy**

```hcl
# ❌ YANLIŞ:
resource "helm_release" "redis" {
  name  = "redis"
  chart = "redis"
}

# ✅ DOĞRU:
# Terraform'da hiçbir app/database deploy etme!
# Sadece ApplicationClaim apply et.
```

### **3. Manuel kubectl**

```bash
# ❌ YANLIŞ:
kubectl apply -f postgres-deployment.yaml
kubectl apply -f redis-statefulset.yaml

# ✅ DOĞRU:
kubectl apply -f ecommerce-claim.yaml
# Operator her şeyi halleder
```

### **4. Hardcoded Values**

```go
// ❌ YANLIŞ:
chartVersion := "13.2.0"  // Hardcoded

// ✅ DOĞRU:
chartVersion := operatorVersions["cloudnative-pg"]  // Configurable
```

### **5. Git-based GitOps**

```go
// ❌ YANLIŞ:
func pushManifestsToGit() {
    // Git push rendered manifests
}

// ✅ DOĞRU:
// Git kullanma! K8s-native (ApplicationSet + ChartMuseum)
```

---

## 📞 WHEN TO ASK

**Şu durumlarda MUTLAKA sor:**

1. ❓ Agreed architecture değişiklik gerektiriyor mu?
2. ❓ Yeni bir dependency/tool eklemek gerekiyor mu?
3. ❓ Operator versiyonu değiştirmek gerekiyor mu?
4. ❓ Terraform yapısında major değişiklik gerekiyor mu?
5. ❓ ApplicationClaim CRD field eklemek gerekiyor mu?
6. ❓ Bir şey local'de çalışmıyor ama neden bilmiyorum?

**SOR, DEĞİŞTİRME!**

---

## ✅ SUCCESS DEFINITION

**Başarılı bir değişiklik:**

1. ✅ Türkçe açıklandı
2. ✅ Agreed architecture'a uygun
3. ✅ Manuel step yok
4. ✅ Hardcoded value yok
5. ✅ Production-ready operators kullanıldı
6. ✅ Orbstack'te test edildi
7. ✅ Commit + push yapıldı
8. ✅ CLAUDE.md/rules.md/workflow.md'ye uygun
9. ✅ Ultra think yapıldı
10. ✅ User onayı alındı (gerekirse)

---

## 🎯 REMEMBER

```
┌─────────────────────────────────────────────────────┐
│  "If you're not sure, ASK!"                         │
│  "If it's not in the claim, it shouldn't exist!"    │
│  "If it's manual, it's wrong!"                      │
│  "If it's Bitnami, it's not production-ready!"      │
│  "If it's not committed, it doesn't exist!"         │
└─────────────────────────────────────────────────────┘
```

---

> **Last Updated:** 2025-12-21
> **Enforcement:** STRICT - NO EXCEPTIONS
> **Violations:** Report immediately
