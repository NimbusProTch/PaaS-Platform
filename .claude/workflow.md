# 🔄 DEVELOPMENT WORKFLOW

> **Platform Operator Development - Step by Step Guide**

---

## 🏁 INITIAL SETUP (Once)

### **1. Clone Repository**

```bash
cd ~/Desktop/Teknokent-Projeler
git clone https://github.com/NimbusProTch/PaaS-Platform.git
cd PaaS-Platform
git checkout feature/custom-platform-operator
```

### **2. Install Dependencies**

```bash
# Go (Operator development)
brew install go@1.22

# Kubernetes tools
brew install kubectl helm

# Terraform/OpenTofu
brew install opentofu

# Development tools
brew install jq yq yamllint
```

### **3. Orbstack Setup**

```bash
# Orbstack provides local Kubernetes cluster
# Check if K8s context available:
kubectl config get-contexts

# Should see:
# * orbstack    orbstack    orbstack

# Set as current:
kubectl config use-context orbstack
```

### **4. Local Kubernetes Components**

```bash
# ArgoCD
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for ArgoCD ready
kubectl wait --for=condition=available --timeout=300s \
  deployment/argocd-server -n argocd

# Get ArgoCD password
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" | base64 -d

# Port forward
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Access: https://localhost:8080
# Username: admin
# Password: <from above>

# ChartMuseum
helm repo add chartmuseum https://chartmuseum.github.io/charts
helm install chartmuseum chartmuseum/chartmuseum \
  -n chartmuseum --create-namespace \
  --set env.open.DISABLE_API=false \
  --set persistence.enabled=true

# Platform Operator (run locally)
cd infrastructure/platform-operator
make install  # Install CRDs
make run      # Run controller locally
```

---

## 💻 DAILY DEVELOPMENT WORKFLOW

### **Morning Checklist**

```bash
# 1. Orbstack running?
# Check Orbstack app in menu bar

# 2. Pull latest changes
git checkout feature/custom-platform-operator
git pull origin feature/custom-platform-operator

# 3. Check K8s cluster
kubectl get nodes
kubectl get pods -A

# 4. Start operator (if needed)
cd infrastructure/platform-operator
make run
```

---

## 🔧 MAKING CHANGES

### **Scenario 1: Update Operator Code**

```bash
# 1. Make changes
vim infrastructure/platform-operator/internal/controller/argocd_controller.go

# 2. Run locally
make run

# 3. Test with claim
kubectl apply -f infrastructure/platform-operator/ecommerce-claim.yaml

# 4. Watch logs
# (Operator logs in terminal where 'make run' is running)

# 5. Verify ApplicationSet created
kubectl get applicationset -n argocd

# 6. Verify Applications generated
kubectl get application -n argocd

# 7. If working, commit + push
git add infrastructure/platform-operator/internal/controller/
git commit -m "feat: Add operator auto-install logic"
git push origin feature/custom-platform-operator
```

### **Scenario 2: Update Chart Templates**

```bash
# 1. Make changes
vim infrastructure/platform-operator/charts/common/templates/platform/postgresql-cluster.yaml

# 2. Package chart
cd infrastructure/platform-operator/charts
helm package common

# Output: common-2.0.0.tgz

# 3. Push to ChartMuseum
curl --data-binary "@common-2.0.0.tgz" \
  http://localhost:8080/api/charts

# (ChartMuseum port-forward needed:)
# kubectl port-forward svc/chartmuseum -n chartmuseum 8080:8080

# 4. Test with ApplicationClaim
kubectl delete applicationclaim ecommerce-qa
kubectl apply -f infrastructure/platform-operator/ecommerce-claim.yaml

# 5. Watch ArgoCD sync
kubectl get application -n argocd -w

# 6. Verify resources
kubectl get cluster -n qa  # PostgreSQL cluster
kubectl get pods -n qa

# 7. If working, commit + push
git add infrastructure/platform-operator/charts/
git commit -m "feat: Add PostgreSQL cluster template"
git push origin feature/custom-platform-operator
```

### **Scenario 3: Update Terraform**

```bash
# 1. Make changes
vim infrastructure/aws/argocd-projects.tf

# 2. Plan (local validation)
cd infrastructure/aws
terraform init
terraform plan

# 3. If plan OK, commit + push
git add infrastructure/aws/
git commit -m "feat: Add AppProjects for team isolation"
git push origin feature/custom-platform-operator

# 4. Apply to real infrastructure (later, when ready)
# terraform apply -var environment=qa
```

---

## 🧪 TESTING WORKFLOW

### **Test 1: Single ApplicationClaim**

```yaml
# test-claim.yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: test-app
spec:
  namespace: test
  environment: dev
  owner:
    team: Test Team
    email: test@example.com
  applications:
    - name: nginx
      image: nginx:latest
      replicas: 1
      ports:
        - port: 80
```

```bash
# Apply
kubectl apply -f test-claim.yaml

# Watch operator logs
# (in terminal where 'make run' is running)

# Check ApplicationSet
kubectl get applicationset -n argocd

# Check Applications
kubectl get application -n argocd

# Check pods
kubectl get pods -n test

# Cleanup
kubectl delete applicationclaim test-app
kubectl delete namespace test
```

### **Test 2: PostgreSQL Component**

```yaml
# postgres-claim.yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: test-postgres
spec:
  namespace: db-test
  environment: dev
  owner:
    team: Test Team
    email: test@example.com
  components:
    - type: postgresql
      name: testdb
      version: "16"
      config:
        replicas: 2
        storage: 10Gi
```

```bash
# Apply
kubectl apply -f postgres-claim.yaml

# Operator should:
# 1. Detect CloudNativePG operator needed
# 2. Install CloudNativePG (if not exists)
# 3. Create ApplicationSet
# 4. ArgoCD creates Application
# 5. ArgoCD renders PostgreSQL Cluster CRD
# 6. CloudNativePG operator creates StatefulSet

# Check operator installation
kubectl get application cloudnative-pg -n argocd
kubectl get pods -n cnpg-system

# Check PostgreSQL cluster
kubectl get cluster -n db-test
kubectl get pods -n db-test

# Cleanup
kubectl delete applicationclaim test-postgres
kubectl delete namespace db-test
```

### **Test 3: Full Stack**

```yaml
# fullstack-claim.yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: test-fullstack
spec:
  namespace: fullstack
  environment: dev
  owner:
    team: Test Team
    email: test@example.com
  applications:
    - name: api
      image: nginx:latest
      replicas: 2
      ports:
        - port: 8080
  components:
    - type: postgresql
      name: db
      version: "16"
      config:
        replicas: 2
        storage: 10Gi
    - type: redis
      name: cache
      version: "7.2"
      config:
        mode: sentinel
        replicas: 3
```

```bash
# Apply
kubectl apply -f fullstack-claim.yaml

# Operator should install:
# - CloudNativePG operator
# - Redis operator

# Then create ApplicationSet with 3 elements:
# - api (microservice)
# - db (postgresql)
# - cache (redis)

# Verify all components
kubectl get all -n fullstack
kubectl get cluster -n fullstack  # PostgreSQL
kubectl get redisfailover -n fullstack  # Redis (if using spotahome operator)

# Cleanup
kubectl delete applicationclaim test-fullstack
kubectl delete namespace fullstack
```

---

## 🐛 DEBUGGING

### **Operator Not Reconciling**

```bash
# 1. Check operator logs
# (terminal where 'make run' is running)

# 2. Check CRD installed
kubectl get crd applicationclaims.platform.infraforge.io

# 3. Check claim status
kubectl get applicationclaim -o yaml

# 4. Check RBAC
kubectl auth can-i create applicationsets --as=system:serviceaccount:platform-operator-system:platform-operator
```

### **ApplicationSet Not Expanding**

```bash
# 1. Check ApplicationSet exists
kubectl get applicationset -n argocd

# 2. Check ApplicationSet status
kubectl get applicationset <name> -n argocd -o yaml

# 3. Check ArgoCD ApplicationSet controller logs
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-applicationset-controller

# 4. Check generator elements
kubectl get applicationset <name> -n argocd -o jsonpath='{.spec.generators[0].list.elements}'
```

### **Chart Not Found**

```bash
# 1. Check ChartMuseum
kubectl get pods -n chartmuseum

# 2. Port forward
kubectl port-forward svc/chartmuseum -n chartmuseum 8080:8080

# 3. List charts
curl http://localhost:8080/api/charts

# 4. If empty, re-push chart
cd infrastructure/platform-operator/charts
helm package common
curl --data-binary "@common-2.0.0.tgz" http://localhost:8080/api/charts
```

### **Operator Auto-Install Failing**

```bash
# 1. Check ArgoCD Application for operator
kubectl get application cloudnative-pg -n argocd -o yaml

# 2. Check sync status
kubectl get application cloudnative-pg -n argocd -o jsonpath='{.status.sync.status}'

# 3. Check health status
kubectl get application cloudnative-pg -n argocd -o jsonpath='{.status.health.status}'

# 4. Check ArgoCD logs
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-application-controller
```

---

## 📦 GIT WORKFLOW

### **Feature Branch**

```bash
# Create feature branch (if not exists)
git checkout -b feature/custom-platform-operator

# Make changes
# ... edit files ...

# Stage changes
git add .

# Commit with convention
git commit -m "feat: Add Redis operator auto-install"

# Push
git push origin feature/custom-platform-operator
```

### **Commit Message Convention**

```
feat:     New feature
fix:      Bug fix
chore:    Dependency/cleanup
docs:     Documentation
refactor: Code refactoring
test:     Tests
```

**Examples:**

```bash
git commit -m "feat: Add CloudNativePG operator auto-install logic"
git commit -m "fix: Fix ApplicationSet project assignment"
git commit -m "chore: Update Go dependencies"
git commit -m "docs: Update CLAUDE.md with Redis operator info"
git commit -m "refactor: Extract operator detection to separate function"
git commit -m "test: Add integration test for PostgreSQL deployment"
```

### **Sync with Main**

```bash
# Get latest main
git checkout main
git pull origin main

# Merge to feature branch
git checkout feature/custom-platform-operator
git merge main

# Resolve conflicts (if any)
# ... edit files ...
git add .
git commit -m "chore: Merge main into feature branch"

# Push
git push origin feature/custom-platform-operator
```

---

## 🚀 DEPLOYMENT TO AWS (When Ready)

### **Pre-Deployment Checklist**

- [ ] All tests pass locally (Orbstack)
- [ ] ApplicationClaim → ApplicationSet → Applications → Resources working
- [ ] Operator auto-install working
- [ ] Chart templates rendering correctly
- [ ] AppProjects configured
- [ ] Terraform plan successful
- [ ] AWS credentials configured
- [ ] GitHub Actions workflows tested

### **Terraform Apply**

```bash
cd infrastructure/aws

# Initialize
terraform init

# Plan
terraform plan -var environment=qa -out=tfplan

# Review plan carefully!

# Apply
terraform apply tfplan

# Outputs
terraform output

# Expected:
# - EKS cluster endpoint
# - ArgoCD URL
# - Kubeconfig command
```

### **Connect to EKS**

```bash
# Update kubeconfig
aws eks update-kubeconfig --name infraforge-qa --region eu-west-1

# Verify
kubectl get nodes
kubectl get pods -A

# Check ArgoCD
kubectl get pods -n argocd

# Check Platform Operator
kubectl get pods -n platform-operator-system

# Check ChartMuseum
kubectl get pods -n chartmuseum
```

### **Deploy ApplicationClaim**

```bash
# Apply claim
kubectl apply -f claims/ecommerce-qa-claim.yaml

# Watch progress
kubectl get applicationclaim -w
kubectl get applicationset -n argocd -w
kubectl get application -n argocd -w
kubectl get pods -n qa -w
```

---

## 📊 MONITORING

### **Operator Metrics**

```bash
# Operator logs
kubectl logs -n platform-operator-system -l control-plane=controller-manager -f

# Operator events
kubectl get events -n platform-operator-system --sort-by='.lastTimestamp'
```

### **ArgoCD Metrics**

```bash
# ArgoCD dashboard
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Open: https://localhost:8080

# CLI
argocd login localhost:8080
argocd app list
argocd app get ecommerce-qa-product-service
argocd app sync ecommerce-qa-product-service
```

### **Resource Status**

```bash
# ApplicationClaims
kubectl get applicationclaim

# ApplicationSets
kubectl get applicationset -n argocd

# Applications
kubectl get application -n argocd

# PostgreSQL clusters
kubectl get cluster -A

# Redis clusters
kubectl get redisfailover -A

# RabbitMQ clusters
kubectl get rabbitmqcluster -A

# All pods
kubectl get pods -A
```

---

## 🔄 DAILY COMMIT ROUTINE

```bash
# End of day checklist:

# 1. Status check
git status

# 2. Stage all changes
git add .

# 3. Commit with meaningful message
git commit -m "feat: Implement operator auto-install for MongoDB"

# 4. Push
git push origin feature/custom-platform-operator

# 5. Verify on GitHub
# https://github.com/NimbusProTch/PaaS-Platform/tree/feature/custom-platform-operator
```

---

## 📞 SUPPORT

**Issues:** https://github.com/NimbusProTch/PaaS-Platform/issues

**Documentation:**
- Architecture: `.claude/CLAUDE.md`
- Rules: `.claude/rules.md`
- Workflow: `.claude/workflow.md` (this file)

---

> **Last Updated:** 2025-12-21
> **Status:** Active Development
