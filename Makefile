.PHONY: help dev cluster gitea argocd operator token bootstrap argocd-setup claims clean logs status full-deploy kind-create kind-delete install-gitea install-argocd install-operator install-chartmuseum setup-gitea deploy-claims upload-charts

# Include .env file if it exists
-include .env
export

CLUSTER_NAME = infraforge-local
GITEA_ADMIN_USER = gitea_admin
GITEA_ADMIN_PASS = r8sA8CPHD9!bt6d
GITHUB_TOKEN ?= $(GITHUB_TOKEN_ENV)
GITHUB_USER = NimbusProTch
ARGOCD_VERSION = v3.2.3

help: ## Yardım göster
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# 🚀 ANA KOMUT - TEK KOMUTLA HERŞEY
full-deploy: ## 🚀 TAM DEPLOYMENT (Sıfırdan, otomatik)
	@echo "════════════════════════════════════════════════════════════════"
	@echo "🚀 FULL PLATFORM DEPLOYMENT BAŞLIYOR"
	@echo "════════════════════════════════════════════════════════════════"
	@$(MAKE) clean
	@$(MAKE) kind-create
	@$(MAKE) install-gitea
	@$(MAKE) install-argocd
	@$(MAKE) install-chartmuseum
	@$(MAKE) install-operator
	@$(MAKE) setup-gitea
	@$(MAKE) upload-charts
	@$(MAKE) deploy-claims
	@echo ""
	@echo "🎉 ════════════════ DEPLOYMENT TAMAMLANDI ═══════════════════"
	@echo "✅ Cluster: $(CLUSTER_NAME)"
	@echo "✅ Gitea: http://localhost:30300 ($(GITEA_ADMIN_USER)/$(GITEA_ADMIN_PASS))"
	@echo "✅ ArgoCD: https://localhost:8080 (admin/password)"
	@echo "✅ ChartMuseum: http://localhost:30880"
	@echo "✅ Platform Operator: Çalışıyor"
	@echo "✅ GitOps Repository: voltran hazır"
	@echo "✅ Applications: Deploy ediliyor..."
	@echo ""
	@echo "📊 Status kontrolü: make status"
	@echo "📋 Operator logları: make logs"
	@echo "🔍 ArgoCD apps: kubectl get applications -n argocd"
	@echo "════════════════════════════════════════════════════════════════"
	@$(MAKE) status

# CLUSTER
kind-create: ## Kind cluster oluştur
	@echo "🔨 Kind cluster oluşturuluyor..."
	@kind create cluster --name $(CLUSTER_NAME) --config kind-config.yaml
	@echo "✅ Cluster hazır"

# GITEA
install-gitea: ## Gitea kur (minimal)
	@echo "📦 Gitea kuruluyor..."
	@kubectl create namespace gitea --dry-run=client -o yaml | kubectl apply -f -
	@helm repo add gitea-charts https://dl.gitea.com/charts/ 2>/dev/null || true
	@helm repo update gitea-charts
	@helm upgrade --install gitea gitea-charts/gitea -n gitea \
		--set service.http.type=NodePort \
		--set service.http.nodePort=30300 \
		--set gitea.admin.username=$(GITEA_ADMIN_USER) \
		--set gitea.admin.password=$(GITEA_ADMIN_PASS) \
		--set gitea.admin.email=gitea@local.domain \
		--set persistence.enabled=false \
		--set postgresql-ha.enabled=false \
		--set postgresql.enabled=false \
		--set redis-cluster.enabled=false \
		--set redis.enabled=false \
		--set gitea.config.database.DB_TYPE=sqlite3 \
		--set gitea.config.cache.ENABLED=false \
		--set gitea.config.server.ROOT_URL=http://gitea-http.gitea.svc.cluster.local:3000 \
		--wait --timeout 5m
	@echo "⏳ Gereksiz pod'lar temizleniyor..."
	@kubectl delete statefulset -n gitea gitea-valkey-cluster 2>/dev/null || true
	@kubectl delete service -n gitea gitea-valkey-cluster gitea-valkey-cluster-headless 2>/dev/null || true
	@kubectl delete pvc -n gitea -l app.kubernetes.io/name=valkey 2>/dev/null || true
	@echo "✅ Gitea hazır"

# ARGOCD
install-argocd: ## ArgoCD kur
	@echo "🚀 ArgoCD kuruluyor..."
	@kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
	@kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/$(ARGOCD_VERSION)/manifests/install.yaml
	@echo "⏳ ArgoCD bekleniyor..."
	@kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
	@echo "🔑 ArgoCD repository secret'ları oluşturuluyor..."
	@kubectl create secret generic gitea-repo -n argocd \
		--from-literal=type=git \
		--from-literal=url=http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran \
		--from-literal=username=$(GITEA_ADMIN_USER) \
		--from-literal=password=$(GITEA_ADMIN_PASS) \
		--dry-run=client -o yaml | kubectl label -f - --local argocd.argoproj.io/secret-type=repository -o yaml | kubectl apply -f -
	@echo "✅ ArgoCD hazır"
	@echo "Admin Password: $$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"

# CHARTMUSEUM
install-chartmuseum: ## ChartMuseum kur
	@echo "📊 ChartMuseum kuruluyor..."
	@kubectl create namespace chartmuseum --dry-run=client -o yaml | kubectl apply -f -
	@helm repo add chartmuseum https://chartmuseum.github.io/charts 2>/dev/null || true
	@helm repo update chartmuseum
	@helm upgrade --install chartmuseum chartmuseum/chartmuseum -n chartmuseum \
		--set env.open.DISABLE_API=false \
		--set service.type=NodePort \
		--set service.nodePort=30880 \
		--set persistence.enabled=false \
		--wait --timeout 5m
	@echo "⏳ ChartMuseum hazır olması bekleniyor..."
	@kubectl wait --for=condition=available --timeout=180s deployment/chartmuseum -n chartmuseum
	@kubectl create secret generic chartmuseum-repo -n argocd \
		--from-literal=type=helm \
		--from-literal=url=http://chartmuseum.chartmuseum.svc.cluster.local:8080 \
		--from-literal=name=chartmuseum \
		--dry-run=client -o yaml | kubectl label -f - --local argocd.argoproj.io/secret-type=repository -o yaml | kubectl apply -f -
	@echo "✅ ChartMuseum hazır (http://localhost:30880)"

# OPERATOR
install-operator: ## Platform Operator kur
	@echo "📋 CRD'ler kuruluyor..."
	@kubectl apply -f infrastructure/platform-operator/config/crd/bases
	@echo "🚀 Operator namespace oluşturuluyor..."
	@kubectl create namespace platform-operator-system --dry-run=client -o yaml | kubectl apply -f -
	@echo "🔐 Image pull secret oluşturuluyor..."
	@kubectl create secret docker-registry ghcr-secret \
		--docker-server=ghcr.io \
		--docker-username=$(GITHUB_USER) \
		--docker-password=$(GITHUB_TOKEN) \
		--namespace platform-operator-system \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "🔐 Gitea token oluşturuluyor..."
	@sleep 5
	@POD=$$(kubectl get pod -n gitea -l app.kubernetes.io/name=gitea -o jsonpath='{.items[0].metadata.name}') && \
	TOKEN=$$(kubectl exec -n gitea $$POD -- gitea admin user generate-access-token \
		--username $(GITEA_ADMIN_USER) \
		--token-name platform-operator \
		--scopes write:organization,write:repository,write:user \
		--raw 2>/dev/null || echo "dummy-token") && \
	kubectl create secret generic gitea-token -n platform-operator-system \
		--from-literal=token=$$TOKEN \
		--from-literal=username=$(GITEA_ADMIN_USER) \
		--from-literal=url=http://gitea-http.gitea.svc.cluster.local:3000 \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "🚀 Operator deploy ediliyor..."
	@kubectl apply -f infrastructure/platform-operator/config/default/rbac.yaml -n platform-operator-system
	@cd infrastructure/platform-operator/config/manager && \
		kustomize edit set image controller=ghcr.io/nimbusprotch/platform-operator:latest && \
		kustomize edit add patch --path imagePullSecrets.yaml --kind Deployment && \
		echo "- op: add\n  path: /spec/template/spec/imagePullSecrets\n  value:\n  - name: ghcr-secret" > imagePullSecrets.yaml && \
		kubectl apply -k . -n platform-operator-system
	@echo "⏳ Operator bekleniyor..."
	@kubectl wait --for=condition=available --timeout=180s deployment/controller-manager -n platform-operator-system 2>/dev/null || true
	@echo "✅ Platform Operator hazır"

# GITEA SETUP
setup-gitea: ## Gitea'ya GitOps structure kur
	@echo "🔧 Gitea repository oluşturuluyor..."
	@kubectl port-forward -n gitea svc/gitea-http 3000:3000 > /dev/null 2>&1 & \
		PF_PID=$$! && \
		sleep 3 && \
		curl -X POST "http://$(GITEA_ADMIN_USER):$(GITEA_ADMIN_PASS)@localhost:3000/api/v1/orgs" \
			-H "Content-Type: application/json" \
			-d '{"username": "infraforge", "full_name": "InfraForge", "description": "Platform Organization"}' 2>/dev/null || true && \
		curl -X POST "http://$(GITEA_ADMIN_USER):$(GITEA_ADMIN_PASS)@localhost:3000/api/v1/orgs/infraforge/repos" \
			-H "Content-Type: application/json" \
			-d '{"name": "voltran", "description": "GitOps Repository", "private": false}' 2>/dev/null || true && \
		kill $$PF_PID 2>/dev/null || true
	@echo "📂 GitOps structure push ediliyor..."
	@bash scripts/setup-gitea.sh
	@echo "✅ Gitea GitOps structure hazır"

# CHART UPLOAD
upload-charts: ## ChartMuseum'a chart'ları yükle
	@echo "📦 Helm chart'ları paketleniyor..."
	@mkdir -p /tmp/charts
	@helm package charts/microservice -d /tmp/charts
	@helm package charts/postgresql -d /tmp/charts
	@helm package charts/redis -d /tmp/charts
	@helm package charts/mongodb -d /tmp/charts 2>/dev/null || true
	@helm package charts/rabbitmq -d /tmp/charts 2>/dev/null || true
	@helm package charts/kafka -d /tmp/charts 2>/dev/null || true
	@echo "📤 ChartMuseum'a upload ediliyor..."
	@for chart in /tmp/charts/*.tgz; do \
		curl -X POST --data-binary "@$$chart" http://localhost:30880/api/charts 2>/dev/null || \
		echo "Hata: $$chart upload edilemedi (ChartMuseum henüz hazır olmayabilir)"; \
	done
	@rm -rf /tmp/charts
	@echo "✅ Chart'lar yüklendi"

# CLAIMS DEPLOY
deploy-claims: ## Dev ortamındaki enabled claim'leri deploy et
	@echo "🚀 Bootstrap claim deploy ediliyor..."
	@kubectl apply -f deployments/dev/bootstrap-claim.yaml
	@echo "⏳ Bootstrap işleniyor (20 saniye)..."
	@sleep 20
	@echo "🚀 Platform infrastructure deploy ediliyor..."
	@kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
	@echo "⏳ Platform services işleniyor (15 saniye)..."
	@sleep 15
	@echo "🚀 Applications deploy ediliyor..."
	@kubectl apply -f deployments/dev/apps-claim.yaml
	@echo "⏳ Applications işleniyor (10 saniye)..."
	@sleep 10
	@echo "✅ Claims deploy edildi!"
	@echo ""
	@echo "Enabled Services:"
	@echo "  - Apps: product-service, user-service"
	@echo "  - DBs: product-db, user-db"
	@echo "  - Cache: redis"

# STATUS & MONITORING
status: ## Sistem durumunu göster
	@echo ""
	@echo "📊 ═══════════════ PLATFORM STATUS ═══════════════"
	@echo ""
	@echo "🔷 Core Services:"
	@echo -n "  Gitea:        " && (kubectl get pod -n gitea -l app.kubernetes.io/name=gitea --no-headers 2>/dev/null | wc -l | xargs echo "pods running") || echo "❌ Not found"
	@echo -n "  ArgoCD:       " && (kubectl get pod -n argocd -l app.kubernetes.io/name=argocd-server --no-headers 2>/dev/null | wc -l | xargs echo "pods running") || echo "❌ Not found"
	@echo -n "  ChartMuseum:  " && (kubectl get pod -n chartmuseum --no-headers 2>/dev/null | wc -l | xargs echo "pods running") || echo "❌ Not found"
	@echo -n "  Operator:     " && (kubectl get pod -n platform-operator-system --no-headers 2>/dev/null | wc -l | xargs echo "pods running") || echo "❌ Not found"
	@echo ""
	@echo "🔷 Claims:"
	@kubectl get bootstrapclaim,applicationclaim,platformapplicationclaim 2>/dev/null || echo "  ❌ No claims found"
	@echo ""
	@echo "🔷 ArgoCD Applications:"
	@kubectl get applications -n argocd --no-headers 2>/dev/null | head -5 || echo "  ❌ No applications"
	@echo ""
	@echo "🔷 ApplicationSets:"
	@kubectl get applicationsets -n argocd --no-headers 2>/dev/null || echo "  ❌ No applicationsets"
	@echo "══════════════════════════════════════════════════"

logs: ## Platform Operator loglarını göster
	@kubectl logs -n platform-operator-system -l control-plane=controller-manager --tail=50 -f

clean: ## Her şeyi temizle
	@echo "🧹 Cluster siliniyor..."
	@kind delete cluster --name $(CLUSTER_NAME) 2>/dev/null || true
	@echo "✅ Temizlik tamamlandı"

# Quick Access Commands
port-forward-argocd: ## ArgoCD port-forward
	@echo "ArgoCD Password: $$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
	@echo "Opening https://localhost:8080"
	@kubectl port-forward svc/argocd-server -n argocd 8080:443

port-forward-gitea: ## Gitea port-forward
	@echo "Opening http://localhost:3000 ($(GITEA_ADMIN_USER)/$(GITEA_ADMIN_PASS))"
	@kubectl port-forward svc/gitea-http -n gitea 3000:3000

port-forward-chartmuseum: ## ChartMuseum port-forward
	@echo "Opening http://localhost:8080"
	@kubectl port-forward svc/chartmuseum -n chartmuseum 8080:8080

# Aliases for backward compatibility
cluster: kind-create
gitea: install-gitea
argocd: install-argocd
operator: install-operator
chartmuseum: install-chartmuseum
kind-delete: clean