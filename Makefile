.PHONY: help dev cluster gitea argocd operator token bootstrap claims clean logs status lightweight

CLUSTER_NAME = platform-dev
GITEA_ADMIN_USER = gitea_admin
GITEA_ADMIN_PASS = r8sA8CPHD9!bt6d
OPERATOR_IMAGE = platform-operator:dev
GITHUB_TOKEN ?= ${GITHUB_TOKEN}
ARGOCD_VERSION = v2.9.3

help: ## Yardım göster
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

dev: clean cluster gitea operator token bootstrap claims ## Tam development ortamı kur
	@echo ""
	@echo "✅ Development ortamı hazır!"
	@echo "🌐 Gitea: http://localhost:30300 ($(GITEA_ADMIN_USER)/$(GITEA_ADMIN_PASS))"
	@echo "📊 Status: make status"
	@echo "📋 Logs: make logs"

cluster: ## Kind cluster oluştur
	@echo "🔨 Kind cluster oluşturuluyor..."
	@kind create cluster --name $(CLUSTER_NAME) --config kind-config.yaml
	@echo "✅ Cluster hazır"

gitea: ## Gitea kur (minimal, TEK pod)
	@echo "📦 Gitea kuruluyor..."
	@kubectl create namespace gitea --dry-run=client -o yaml | kubectl apply -f -
	@helm repo add gitea-charts https://dl.gitea.com/charts/ 2>/dev/null || true
	@helm repo update gitea-charts 2>/dev/null
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
	  --wait --timeout 5m 2>/dev/null
	@echo "⏳ Valkey temizleniyor..."
	@sleep 3
	@kubectl delete statefulset -n gitea gitea-valkey-cluster 2>/dev/null || true
	@kubectl delete service -n gitea gitea-valkey-cluster gitea-valkey-cluster-headless 2>/dev/null || true
	@kubectl delete pvc -n gitea -l app.kubernetes.io/name=valkey 2>/dev/null || true
	@echo "✅ Gitea hazır (TEK pod)"

argocd: ## ArgoCD kur
	@echo "🚀 ArgoCD kuruluyor..."
	@kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
	@kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/$(ARGOCD_VERSION)/manifests/install.yaml
	@echo "⏳ ArgoCD pod'ların hazır olması bekleniyor..."
	@kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
	@echo "🔑 ArgoCD admin şifresi alınıyor..."
	@echo ""
	@echo "ArgoCD Admin Password:"
	@kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d && echo ""
	@echo ""
	@echo "✅ ArgoCD hazır!"
	@echo "Port-forward: kubectl port-forward svc/argocd-server -n argocd 8080:443"
	@echo "Login: admin / (yukarıdaki şifre)"

operator: ## Operator build ve deploy
	@echo "🔨 Operator build ediliyor..."
	@docker build -t $(OPERATOR_IMAGE) -f infrastructure/platform-operator/Dockerfile infrastructure/platform-operator -q
	@echo "📦 Kind'a yükleniyor..."
	@kind load docker-image $(OPERATOR_IMAGE) --name $(CLUSTER_NAME)
	@echo "📋 CRD kuruluyor..."
	@kubectl apply -f infrastructure/platform-operator/config/crd/bases
	@echo "🚀 Operator deploy ediliyor..."
	@kubectl create namespace platform-operator-system --dry-run=client -o yaml | kubectl apply -f -
	@kubectl apply -f infrastructure/platform-operator/config/default/rbac.yaml -n platform-operator-system
	@cd infrastructure/platform-operator/config/manager && \
	  kustomize edit set image controller=$(OPERATOR_IMAGE) && \
	  kubectl apply -k . -n platform-operator-system
	@echo "⏳ Operator bekleniyor..."
	@sleep 10
	@echo "✅ Operator hazır"

token: ## Gitea ve GitHub token oluştur
	@echo "🔑 Gitea token oluşturuluyor..."
	@sleep 5
	@POD=$$(kubectl get pod -n gitea -l app.kubernetes.io/name=gitea -o jsonpath='{.items[0].metadata.name}' 2>/dev/null) && \
	TOKEN=$$(kubectl exec -n gitea $$POD -- gitea admin user generate-access-token \
	  --username $(GITEA_ADMIN_USER) \
	  --token-name platform-operator \
	  --scopes write:organization,write:repository,write:user \
	  --raw 2>/dev/null) && \
	kubectl create secret generic gitea-token -n platform-operator-system \
	  --from-literal=token=$$TOKEN \
	  --from-literal=username=$(GITEA_ADMIN_USER) \
	  --from-literal=url=http://gitea-http.gitea.svc.cluster.local:3000 \
	  --dry-run=client -o yaml | kubectl apply -f -
	@echo "🔑 GitHub token oluşturuluyor..."
	@kubectl create secret generic github-token -n platform-operator-system \
	  --from-literal=token=$(GITHUB_TOKEN) \
	  --dry-run=client -o yaml | kubectl apply -f -
	@kubectl delete pod -n platform-operator-system -l control-plane=controller-manager 2>/dev/null || true
	@echo "✅ Token'lar hazır, operator yeniden başlatıldı"

bootstrap: ## Bootstrap deploy et
	@echo "🚀 Bootstrap deploy ediliyor..."
	@kubectl apply -f infrastructure/platform-operator/bootstrap-claim.yaml
	@echo "⏳ Bootstrap bekleniyor..."
	@sleep 15
	@echo "✅ Bootstrap tamamlandı"

claims: ## Application ve Platform claims deploy et
	@echo "🚀 Application claims deploy ediliyor..."
	@kubectl apply -f deployments/dev/apps-claim.yaml
	@echo "⏳ Bekleniyor..."
	@sleep 10
	@echo "🚀 Platform claims deploy ediliyor..."
	@kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml
	@echo "⏳ Bekleniyor..."
	@sleep 10
	@echo "✅ Claims tamamlandı"

lightweight: ## Lightweight claims deploy et (2 app + postgres + redis)
	@echo "🚀 Lightweight deployment başlatılıyor..."
	@kubectl apply -f deployments/lightweight/platform-minimal.yaml
	@echo "⏳ Platform services bekleniyor..."
	@sleep 15
	@kubectl apply -f deployments/lightweight/apps-minimal.yaml
	@echo "⏳ Applications bekleniyor..."
	@sleep 10
	@echo "✅ Lightweight deployment tamamlandı"
	@kubectl get applicationclaim,platformapplicationclaim

status: ## Status göster
	@echo "📊 === CLUSTER STATUS ==="
	@echo ""
	@echo "Gitea Pods:"
	@kubectl get pods -n gitea 2>/dev/null || echo "Yok"
	@echo ""
	@echo "Operator Pods:"
	@kubectl get pods -n platform-operator-system 2>/dev/null || echo "Yok"
	@echo ""
	@echo "Bootstrap:"
	@kubectl get bootstrapclaim 2>/dev/null || echo "Yok"
	@echo ""
	@echo "Application Claims:"
	@kubectl get applicationclaim 2>/dev/null || echo "Yok"
	@echo ""
	@echo "Platform Claims:"
	@kubectl get platformapplicationclaim 2>/dev/null || echo "Yok"

logs: ## Operator logları göster
	@kubectl logs -n platform-operator-system -l control-plane=controller-manager --tail=100 -f

clean: ## Her şeyi sil
	@echo "🧹 Temizleniyor..."
	@kind delete cluster --name $(CLUSTER_NAME) 2>/dev/null || true
	@echo "✅ Temizlendi"
