# InfraForge Platform - Local Development Makefile

CLUSTER_NAME ?= platform-test
OPERATOR_IMG ?= platform-operator:latest

.PHONY: help
help: ## Display this help
	@echo "InfraForge Platform - Local Development Commands"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Development Environment

.PHONY: dev-up
dev-up: cluster-up gitea-deploy argocd-deploy operator-deploy ## 🚀 Complete local setup (cluster + gitea + argocd + operator)
	@echo ""
	@echo "✅ Development environment ready!"
	@echo ""
	@echo "Access:"
	@echo "  Gitea:  http://localhost:3000 (admin: gitea_admin / r8sA8CPHD9!bt6d)"
	@echo "  ArgoCD: kubectl port-forward svc/argocd-server -n argocd 8081:443"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Create a BootstrapClaim: kubectl apply -f infrastructure/platform-operator/ecommerce-claim.yaml"
	@echo "  2. Watch operator logs: make logs"
	@echo "  3. Check Gitea repos: open http://localhost:3000"

.PHONY: dev-down
dev-down: cluster-down ## 🗑️  Delete local environment completely
	@echo "✅ Development environment deleted"

.PHONY: dev-restart
dev-restart: dev-down dev-up ## 🔄 Restart entire development environment

##@ Cluster Management

.PHONY: cluster-up
cluster-up: ## Create Kind cluster
	@./scripts/kind-cluster-up.sh $(CLUSTER_NAME)

.PHONY: cluster-down
cluster-down: ## Delete Kind cluster
	@./scripts/kind-cluster-down.sh $(CLUSTER_NAME)

.PHONY: cluster-info
cluster-info: ## Show cluster information
	@kubectl cluster-info --context kind-$(CLUSTER_NAME)
	@echo ""
	@kubectl get nodes

##@ Component Deployment

.PHONY: gitea-deploy
gitea-deploy: ## Deploy Gitea
	@./scripts/deploy-gitea.sh

.PHONY: argocd-deploy
argocd-deploy: ## Deploy ArgoCD
	@./scripts/deploy-argocd.sh

.PHONY: operator-deploy
operator-deploy: ## Build and deploy platform operator
	@./scripts/deploy-operator.sh $(CLUSTER_NAME) $(OPERATOR_IMG)

.PHONY: operator-redeploy
operator-redeploy: ## Rebuild and redeploy operator only
	@echo "🔄 Redeploying operator..."
	@./scripts/deploy-operator.sh $(CLUSTER_NAME) $(OPERATOR_IMG)

##@ Testing

.PHONY: test-bootstrap
test-bootstrap: ## Create BootstrapClaim for testing
	@echo "📝 Creating BootstrapClaim..."
	@kubectl apply -f infrastructure/platform-operator/ecommerce-claim.yaml
	@echo "✅ BootstrapClaim created"
	@echo ""
	@echo "Watch progress:"
	@echo "  kubectl get bootstrapclaim -A -w"

.PHONY: test-app
test-app: ## Create ApplicationClaim for testing
	@echo "📝 Creating ApplicationClaim..."
	@cat <<EOF | kubectl apply -f -
	apiVersion: platform.infraforge.dev/v1
	kind: ApplicationClaim
	metadata:
	  name: ecommerce-dev
	  namespace: default
	spec:
	  environment: development
	  clusterType: dev
	  applications:
	    - name: api
	      chart:
	        name: ecommerce-api
	        source: git
	      image:
	        repository: ecommerce-api
	        tag: latest
	      replicas: 2
	  owner:
	    team: ecommerce
	    email: ecommerce@infraforge.dev
	EOF
	@echo "✅ ApplicationClaim created"

.PHONY: test-platform
test-platform: ## Create PlatformClaim for testing
	@echo "📝 Creating PlatformClaim..."
	@cat <<EOF | kubectl apply -f -
	apiVersion: platform.infraforge.dev/v1
	kind: PlatformClaim
	metadata:
	  name: ecommerce-dev-platform
	  namespace: default
	spec:
	  environment: development
	  clusterType: dev
	  services:
	    - name: postgresql
	      type: database
	      chart:
	        name: postgresql
	        source: platform
	      size: small
	  owner:
	    team: ecommerce
	    email: ecommerce@infraforge.dev
	EOF
	@echo "✅ PlatformClaim created"

##@ Monitoring & Logs

.PHONY: logs
logs: ## Tail operator logs
	@echo "📋 Tailing operator logs (Ctrl+C to exit)..."
	@kubectl logs -n platform-operator-system -l control-plane=controller-manager -f --tail=50

.PHONY: status
status: ## Show status of all components
	@echo "=== Cluster Status ==="
	@kubectl get nodes
	@echo ""
	@echo "=== Gitea ==="
	@kubectl get pods -n gitea
	@echo ""
	@echo "=== ArgoCD ==="
	@kubectl get pods -n argocd
	@echo ""
	@echo "=== Platform Operator ==="
	@kubectl get pods -n platform-operator-system
	@echo ""
	@echo "=== Custom Resources ==="
	@kubectl get bootstrapclaim -A
	@kubectl get applicationclaim -A
	@kubectl get platformclaim -A

.PHONY: watch-claims
watch-claims: ## Watch all custom resource claims
	@echo "👀 Watching all claims (Ctrl+C to exit)..."
	@watch -n 2 'kubectl get bootstrapclaim,applicationclaim,platformclaim -A'

##@ Port Forwarding

.PHONY: gitea-port-forward
gitea-port-forward: ## Port-forward Gitea (localhost:3000)
	@echo "🌐 Port-forwarding Gitea to localhost:3000..."
	@kubectl port-forward -n gitea svc/gitea-http 3000:3000

.PHONY: argocd-port-forward
argocd-port-forward: ## Port-forward ArgoCD (localhost:8081)
	@echo "🌐 Port-forwarding ArgoCD to localhost:8081..."
	@echo "Get admin password:"
	@echo "  kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
	@echo ""
	@kubectl port-forward -n argocd svc/argocd-server 8081:443

##@ Cleanup

.PHONY: clean-claims
clean-claims: ## Delete all custom resource claims
	@echo "🗑️  Deleting all claims..."
	@kubectl delete bootstrapclaim --all -A
	@kubectl delete applicationclaim --all -A
	@kubectl delete platformclaim --all -A
	@echo "✅ All claims deleted"

.PHONY: clean-operator
clean-operator: ## Uninstall operator and CRDs
	@echo "🗑️  Uninstalling operator..."
	@cd infrastructure/platform-operator && make undeploy || true
	@cd infrastructure/platform-operator && make uninstall || true
	@echo "✅ Operator uninstalled"

##@ Operator Development

.PHONY: operator-build
operator-build: ## Build operator image
	@echo "🔨 Building operator..."
	@docker build -f infrastructure/platform-operator/Dockerfile -t $(OPERATOR_IMG) .

.PHONY: operator-load
operator-load: operator-build ## Load operator image into Kind cluster
	@echo "📦 Loading operator image into Kind..."
	@kind load docker-image $(OPERATOR_IMG) --name $(CLUSTER_NAME)

.PHONY: operator-run-local
operator-run-local: ## Run operator locally (outside cluster)
	@echo "🏃 Running operator locally..."
	@cd infrastructure/platform-operator && make run

.PHONY: operator-test
operator-test: ## Run operator tests
	@echo "🧪 Running operator tests..."
	@cd infrastructure/platform-operator && make test

.PHONY: operator-manifests
operator-manifests: ## Generate operator manifests
	@echo "📋 Generating manifests..."
	@cd infrastructure/platform-operator && make manifests

##@ Quick Actions

.PHONY: quick-test
quick-test: test-bootstrap ## Quick test: Create BootstrapClaim and watch logs
	@echo ""
	@echo "Waiting 5 seconds for reconciliation to start..."
	@sleep 5
	@make logs

.PHONY: full-test
full-test: dev-up test-bootstrap ## Full test: Setup everything and create BootstrapClaim
	@echo ""
	@echo "✅ Full test environment ready!"
	@echo "Watch logs with: make logs"
