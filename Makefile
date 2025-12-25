.PHONY: help dev cluster gitea operator token bootstrap clean logs status

CLUSTER_NAME = platform-dev
GITEA_ADMIN_USER = gitea_admin
GITEA_ADMIN_PASS = r8sA8CPHD9!bt6d
OPERATOR_IMAGE = platform-operator:dev

help: ## Yardım göster
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

dev: clean cluster gitea operator token bootstrap ## Tam development ortamı kur
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

operator: ## Operator build ve deploy
	@echo "🔨 Operator build ediliyor..."
	@cd infrastructure/platform-operator && docker build -t $(OPERATOR_IMAGE) -f Dockerfile . -q
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

token: ## Gitea token oluştur
	@echo "🔑 Token oluşturuluyor..."
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
	  --dry-run=client -o yaml | kubectl apply -f - && \
	kubectl delete pod -n platform-operator-system -l control-plane=controller-manager 2>/dev/null || true
	@echo "✅ Token hazır, operator yeniden başlatıldı"

bootstrap: ## Bootstrap deploy et
	@echo "🚀 Bootstrap deploy ediliyor..."
	@kubectl apply -f infrastructure/platform-operator/bootstrap-claim.yaml
	@echo "⏳ Bootstrap bekleniyor..."
	@sleep 10
	@echo "✅ Bootstrap tamamlandı"

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

logs: ## Operator logları göster
	@kubectl logs -n platform-operator-system -l control-plane=controller-manager --tail=100 -f

clean: ## Her şeyi sil
	@echo "🧹 Temizleniyor..."
	@kind delete cluster --name $(CLUSTER_NAME) 2>/dev/null || true
	@echo "✅ Temizlendi"
