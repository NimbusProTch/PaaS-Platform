.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

# Variables
AWS_REGION ?= eu-west-1
ENVIRONMENT ?= dev
CLUSTER_NAME = infraforge-$(ENVIRONMENT)

# Local Development
.PHONY: local-setup
local-setup: ## Setup local Kind cluster
	@echo "🚀 Setting up local Kind cluster..."
	./setup.sh

.PHONY: local-destroy
local-destroy: ## Destroy local Kind cluster
	@echo "🔥 Destroying Kind cluster..."
	kind delete cluster --name infraforge-cluster

# AWS Infrastructure
.PHONY: aws-init
aws-init: ## Initialize OpenTofu/Terraform for AWS
	@echo "📦 Initializing OpenTofu..."
	cd infrastructure/aws && tofu init

.PHONY: aws-plan
aws-plan: ## Plan AWS infrastructure changes
	@echo "📋 Planning AWS infrastructure..."
	cd infrastructure/aws && tofu plan -var-file=environments/$(ENVIRONMENT).tfvars

.PHONY: aws-apply
aws-apply: ## Apply AWS infrastructure
	@echo "🔨 Applying AWS infrastructure..."
	cd infrastructure/aws && tofu apply -var-file=environments/$(ENVIRONMENT).tfvars -auto-approve

.PHONY: aws-destroy
aws-destroy: ## Destroy AWS infrastructure
	@echo "💥 Destroying AWS infrastructure..."
	cd infrastructure/aws && tofu destroy -var-file=environments/$(ENVIRONMENT).tfvars -auto-approve

.PHONY: aws-kubeconfig
aws-kubeconfig: ## Update kubeconfig for AWS EKS cluster
	@echo "📝 Updating kubeconfig..."
	aws eks update-kubeconfig --name $(CLUSTER_NAME) --region $(AWS_REGION)

# Platform Installation
.PHONY: install-operators
install-operators: ## Install all operators on current cluster
	@echo "⚙️ Installing operators..."
	kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.23/releases/cnpg-1.23.0.yaml
	kubectl apply -f https://github.com/rabbitmq/cluster-operator/releases/latest/download/cluster-operator.yml
	helm repo add ot-helm https://ot-container-kit.github.io/helm-charts/ || true
	helm repo update
	helm install redis-operator ot-helm/redis-operator --create-namespace --namespace redis-operator-system
	kubectl apply -k 'https://github.com/minio/operator/resources?ref=v6.0.0'

.PHONY: install-argocd
install-argocd: ## Install ArgoCD
	@echo "🔄 Installing ArgoCD..."
	kubectl create namespace infraforge-argocd || true
	kubectl apply -n infraforge-argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

.PHONY: install-kratix
install-kratix: ## Install Kratix
	@echo "🎯 Installing Kratix..."
	kubectl apply -f https://github.com/syntasso/kratix/releases/latest/download/kratix.yaml
	kubectl create namespace kratix-platform-system || true
	kubectl apply -f kratix/promises/platform-promise.yaml

.PHONY: install-backstage
install-backstage: ## Install Backstage
	@echo "🎭 Installing Backstage..."
	helm repo add backstage https://backstage.github.io/charts || true
	helm repo update
	helm install backstage backstage/backstage \
		--namespace backstage \
		--create-namespace \
		--values backstage/values.yaml \
		--set backstage.image.tag=latest

.PHONY: deploy-platform
deploy-platform: install-operators install-argocd install-kratix ## Deploy complete platform
	@echo "🚀 Deploying platform..."
	kubectl apply -f manifests/platform-cluster/appsets/$(ENVIRONMENT)/operator.yaml
	@echo "✅ Platform deployed!"

# Status and Monitoring
.PHONY: status
status: ## Show platform status
	@echo "📊 Platform Status"
	@kubectl get nodes
	@kubectl get pods -A | grep -E "demo-"

# Development
.PHONY: dev-setup
dev-setup: local-setup deploy-platform ## Complete dev environment setup
	@echo "✅ Development environment ready!"
