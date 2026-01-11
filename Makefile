.PHONY: help full-deploy kind-create kind-delete install-gitea install-argocd install-operator create-gitea-repos deploy-claims status logs clean port-forward-argocd port-forward-gitea cluster gitea argocd operator

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
	@$(MAKE) install-operator
	@$(MAKE) create-gitea-repos
	@$(MAKE) deploy-claims
	@echo ""
	@echo "🎉 ════════════════ DEPLOYMENT TAMAMLANDI ═══════════════════"
	@echo "✅ Cluster: $(CLUSTER_NAME)"
	@echo "✅ Gitea: http://localhost:30300 ($(GITEA_ADMIN_USER)/$(GITEA_ADMIN_PASS))"
	@echo "✅ ArgoCD: https://localhost:8080 (admin/password)"
	@echo "✅ Platform Operator: Çalışıyor"
	@echo "✅ GitOps Repository: voltran hazır"
	@echo "✅ Charts Repository: charts hazır"
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
	@kubectl create secret generic gitea-charts-repo -n argocd \
		--from-literal=type=git \
		--from-literal=url=http://gitea-http.gitea.svc.cluster.local:3000/infraforge/charts \
		--from-literal=username=$(GITEA_ADMIN_USER) \
		--from-literal=password=$(GITEA_ADMIN_PASS) \
		--dry-run=client -o yaml | kubectl label -f - --local argocd.argoproj.io/secret-type=repository -o yaml | kubectl apply -f -
	@echo "✅ ArgoCD hazır"
	@echo "Admin Password: $$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"

# OPERATOR
install-operator: ## Platform Operator kur
	@echo "📋 CRD'ler kuruluyor..."
	@kubectl apply -f infrastructure/platform-operator/config/crd/bases
	@echo "🚀 Operator namespace oluşturuluyor..."
	@kubectl create namespace platform-operator-system --dry-run=client -o yaml | kubectl apply -f -
	@if [ -n "$(GITHUB_TOKEN)" ]; then \
		echo "🔐 Image pull secret oluşturuluyor..."; \
		kubectl create secret docker-registry ghcr-secret \
			--docker-server=ghcr.io \
			--docker-username=$(GITHUB_USER) \
			--docker-password=$(GITHUB_TOKEN) \
			--namespace platform-operator-system \
			--dry-run=client -o yaml | kubectl apply -f -; \
		echo "🔐 GitHub token secret oluşturuluyor..."; \
		kubectl create secret generic github-token \
			--from-literal=token=$(GITHUB_TOKEN) \
			--namespace platform-operator-system \
			--dry-run=client -o yaml | kubectl apply -f -; \
	else \
		echo "⚠️  GITHUB_TOKEN yok, public image kullanılacak"; \
	fi
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
		kustomize edit add patch --path imagePullSecrets.yaml --kind Deployment 2>/dev/null || true && \
		echo "- op: add\n  path: /spec/template/spec/imagePullSecrets\n  value:\n  - name: ghcr-secret" > imagePullSecrets.yaml && \
		kubectl apply -k . -n platform-operator-system
	@echo "⏳ Operator bekleniyor..."
	@kubectl wait --for=condition=available --timeout=180s deployment/controller-manager -n platform-operator-system 2>/dev/null || true
	@echo "✅ Platform Operator hazır"

# GITEA REPOS OLUŞTUR
create-gitea-repos: ## Gitea'da organization ve repository oluştur
	@echo "🔧 Gitea organization ve repository oluşturuluyor..."
	@kubectl port-forward -n gitea svc/gitea-http 3000:3000 > /dev/null 2>&1 & \
		PF_PID=$$! && \
		sleep 3 && \
		curl -X POST "http://$(GITEA_ADMIN_USER):$(GITEA_ADMIN_PASS)@localhost:3000/api/v1/orgs" \
			-H "Content-Type: application/json" \
			-d '{"username": "infraforge", "full_name": "InfraForge", "description": "Platform Organization"}' 2>/dev/null || true && \
		curl -X POST "http://$(GITEA_ADMIN_USER):$(GITEA_ADMIN_PASS)@localhost:3000/api/v1/orgs/infraforge/repos" \
			-H "Content-Type: application/json" \
			-d '{"name": "voltran", "description": "GitOps Repository", "private": false, "auto_init": true}' 2>/dev/null || true && \
		curl -X POST "http://$(GITEA_ADMIN_USER):$(GITEA_ADMIN_PASS)@localhost:3000/api/v1/orgs/infraforge/repos" \
			-H "Content-Type: application/json" \
			-d '{"name": "charts", "description": "Helm Charts Repository", "private": false, "auto_init": true}' 2>/dev/null || true && \
		kill $$PF_PID 2>/dev/null || true
	@echo "✅ Gitea repos hazır (infraforge/voltran, infraforge/charts)"

# CLAIMS DEPLOY
deploy-claims: ## Dev ortamındaki enabled claim'leri deploy et
	@echo "🚀 Bootstrap claim deploy ediliyor..."
	@kubectl apply -f deployments/dev/bootstrap-claim.yaml
	@echo "⏳ Bootstrap işleniyor (30 saniye)..."
	@sleep 30
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

# Aliases for backward compatibility
cluster: kind-create
gitea: install-gitea
argocd: install-argocd
operator: install-operator
kind-delete: clean
