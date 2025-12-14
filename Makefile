# InfraForge - Enterprise Platform as a Service
# Build, Deploy, Scale with Confidence
.PHONY: all clean cluster install-base install-operators install-platform build-image push-image deploy test help check-requirements verify-platform

# Enable strict error handling
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

# InfraForge Configuration
PLATFORM_NAME := infraforge
CLUSTER_NAME ?= $(PLATFORM_NAME)-cluster
DOCKER_REGISTRY ?= docker.io/gaskin23
IMAGE_NAME ?= $(PLATFORM_NAME)-generator
IMAGE_TAG ?= latest
FULL_IMAGE := $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Component versions (configurable)
CERT_MANAGER_VERSION ?= v1.14.5
KRATIX_VERSION ?= v0.89.0
ARGOCD_VERSION ?= v2.10.0

# Check if local secret files exist, if not create from templates
check-secrets:
	@if [ ! -f infrastructure/argocd/github-repo-secret.yaml ]; then \
		echo "Creating github-repo-secret.yaml from template..."; \
		GITHUB_TOKEN_BASE64=$$(echo -n "$${GITHUB_TOKEN}" | base64); \
		export GITHUB_TOKEN_BASE64; \
		envsubst < infrastructure/argocd/github-repo-secret.yaml.template > infrastructure/argocd/github-repo-secret.yaml; \
	fi
	@if [ ! -f infrastructure/kratix/github-credentials.yaml ]; then \
		echo "Creating github-credentials.yaml from template..."; \
		GITHUB_TOKEN_BASE64=$$(echo -n "$${GITHUB_TOKEN}" | base64); \
		export GITHUB_TOKEN_BASE64; \
		envsubst < infrastructure/kratix/github-credentials.yaml.template > infrastructure/kratix/github-credentials.yaml; \
	fi

# InfraForge namespaces
ARGOCD_NAMESPACE := $(PLATFORM_NAME)-argocd
KRATIX_NAMESPACE := kratix-platform-system
OPERATORS_NAMESPACE := $(PLATFORM_NAME)-operators

# Timeouts
WAIT_TIMEOUT := 300s
ROLLOUT_TIMEOUT := 600s

# Colors
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
BLUE := \033[0;34m
NC := \033[0m # No Color

help: ## Show this help message
	@echo ''
	@echo '${BLUE}╦╔╗╔╔═╗╦═╗╔═╗╔═╗╔═╗╦═╗╔═╗╔═╗${NC}'
	@echo '${BLUE}║║║║╠╣ ╠╦╝╠═╣╠╣ ║ ║╠╦╝║ ╦║╣ ${NC}'
	@echo '${BLUE}╩╝╚╝╚  ╩╚═╩ ╩╚  ╚═╝╩╚═╚═╝╚═╝${NC}'
	@echo '${GREEN}Enterprise Platform as a Service${NC}'
	@echo ''
	@echo 'Usage: make [target]'
	@echo ''
	@echo '${YELLOW}Main Targets:${NC}'
	@echo '  ${GREEN}all${NC}                      Full InfraForge setup from scratch'
	@echo '  ${GREEN}clean${NC}                    Delete InfraForge cluster and clean resources'
	@echo ''
	@echo '${YELLOW}Setup Targets:${NC}'
	@echo '  ${BLUE}cluster${NC}                  Create InfraForge Kind cluster'
	@echo '  ${BLUE}install-base${NC}             Install cert-manager and Kratix'
	@echo '  ${BLUE}install-argocd${NC}           Install ArgoCD'
	@echo '  ${BLUE}install-platform${NC}         Install InfraForge platform components'
	@echo ''
	@echo '${YELLOW}Development Targets:${NC}'
	@echo '  ${BLUE}build-image${NC}              Build InfraForge pipeline Docker image'
	@echo '  ${BLUE}push-image${NC}               Push InfraForge image to registry'
	@echo '  ${BLUE}test${NC}                     Create test nginx deployment'
	@echo ''
	@echo '${YELLOW}Utility Targets:${NC}'
	@echo '  ${BLUE}status${NC}                   Show InfraForge platform status'
	@echo '  ${BLUE}logs${NC}                     Show InfraForge pipeline logs'
	@echo '  ${BLUE}port-forward-argocd${NC}      Access ArgoCD UI'
	@echo ''

# Default target
.DEFAULT_GOAL := help

# Check for required tools
check-requirements: ## Check required tools are installed
	@echo "${YELLOW}Checking requirements...${NC}"
	@which docker >/dev/null || (echo "${RED}❌ Docker is required but not installed${NC}" && exit 1)
	@which kind >/dev/null || (echo "${RED}❌ Kind is required but not installed${NC}" && exit 1)
	@which kubectl >/dev/null || (echo "${RED}❌ kubectl is required but not installed${NC}" && exit 1)
	@which helm >/dev/null || (echo "${RED}❌ Helm is required but not installed${NC}" && exit 1)
	@echo "${GREEN}✅ All requirements satisfied${NC}"

all: check-requirements clean cluster install-base install-argocd install-platform ## Full InfraForge setup from scratch

clean: ## Delete InfraForge cluster and clean resources
	@echo "${YELLOW}🧹 Cleaning InfraForge resources...${NC}"
	@echo "Removing Kind cluster: $(CLUSTER_NAME)"
	-@kind delete cluster --name $(CLUSTER_NAME) 2>/dev/null || true
	@echo "Cleaning Docker containers..."
	-@docker rm -f $$(docker ps -aq --filter label=io.x-k8s.kind.cluster=$(CLUSTER_NAME)) 2>/dev/null || true
	@echo "Cleaning Docker volumes..."
	-@docker volume prune -f 2>/dev/null || true
	@echo "${GREEN}✅ Cleanup complete${NC}"

cluster: check-requirements ## Create InfraForge Kind cluster with enterprise config
	@echo "${YELLOW}🚀 Creating InfraForge cluster...${NC}"
	@kind create cluster --name $(CLUSTER_NAME) --config kind-config.yaml
	@kubectl cluster-info --context kind-$(CLUSTER_NAME)
	@echo "${GREEN}✅ InfraForge cluster created successfully${NC}"

install-base: ## Install cert-manager and Kratix for InfraForge
	@echo "${YELLOW}📦 Installing InfraForge base components...${NC}"
	@echo "Installing cert-manager $(CERT_MANAGER_VERSION)..."
	@kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_VERSION)/cert-manager.yaml
	@kubectl wait --for=condition=available --timeout=$(WAIT_TIMEOUT) -n cert-manager deployment --all
	@echo "${GREEN}✅ cert-manager installed${NC}"
	
	@echo "Installing Kratix $(KRATIX_VERSION)..."
	@kubectl apply -f https://github.com/syntasso/kratix/releases/download/$(KRATIX_VERSION)/kratix.yaml
	@kubectl wait --for=condition=available --timeout=$(WAIT_TIMEOUT) -n $(KRATIX_NAMESPACE) deployment --all
	@echo "${GREEN}✅ Kratix installed${NC}"
	
	@echo "${GREEN}✅ Base components ready${NC}"

install-argocd: ## Install ArgoCD for InfraForge
	@echo "${YELLOW}🚀 Installing ArgoCD for InfraForge...${NC}"
	@echo "Adding ArgoCD Helm repository..."
	@helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
	@helm repo update
	
	@echo "Creating namespace..."
	@kubectl create namespace $(ARGOCD_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	
	@echo "Installing ArgoCD with Helm..."
	@helm upgrade --install argocd argo/argo-cd \
		--namespace $(ARGOCD_NAMESPACE) \
		--version 7.1.3 \
		--set configs.params."server\.insecure"=true \
		--set configs.cm.url="https://argocd-server.$(ARGOCD_NAMESPACE).svc.cluster.local:443" \
		--set configs.cm."timeout\.reconciliation"="180s" \
		--wait
	
	@echo "${GREEN}✅ ArgoCD installed${NC}"

install-platform: check-secrets ## Install InfraForge platform components
	@echo "${YELLOW}⚡ Installing InfraForge platform...${NC}"
	
	@if [ -z "$${GITHUB_USERNAME}" ] || [ -z "$${GITHUB_TOKEN}" ]; then \
		echo "${RED}❌ GITHUB_USERNAME and GITHUB_TOKEN environment variables are required${NC}"; \
		echo "${YELLOW}Example: export GITHUB_USERNAME=your-username${NC}"; \
		echo "${YELLOW}Example: export GITHUB_TOKEN=your-personal-access-token${NC}"; \
		exit 1; \
	fi
	
	@echo "Setting up ArgoCD (RBAC, Projects, Repository)..."
	@envsubst < infrastructure/argocd/argocd-setup.yaml | kubectl apply -f -
	
	@echo "Creating GitHub credentials..."
	@kubectl apply -f infrastructure/kratix/github-credentials.yaml
	
	@echo "Applying Kratix RBAC patch for InfraForge..."
	@kubectl apply -f infrastructure/kratix/kratix-rbac-patch.yaml
	
	@echo "Setting up GitHub State Store..."
	@kubectl apply -f infrastructure/kratix/github-state-store.yaml
	
	@echo "Setting up Destination..."
	@kubectl apply -f infrastructure/kratix/destination.yaml
	
	@echo "Setting up InfraForge Promise..."
	@kubectl apply -f infrastructure/kratix/infraforge-promise.yaml
	
	@echo "Setting up NonProd Root Application..."
	@kubectl apply -f infrastructure/argocd/nonprod-root.yaml
	
	@echo "${GREEN}✅ InfraForge platform installed${NC}"

build-image: ## Build InfraForge pipeline Docker image
	@echo "${YELLOW}🔨 Building InfraForge image...${NC}"
	@docker build -f Dockerfile.generator -t $(IMAGE_NAME):$(IMAGE_TAG) .
	@docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(FULL_IMAGE)
	@echo "${GREEN}✅ Image built: $(FULL_IMAGE)${NC}"

push-image: build-image ## Push InfraForge image to registry
	@echo "${YELLOW}📤 Pushing image to registry...${NC}"
	@docker push $(FULL_IMAGE)
	@echo "${GREEN}✅ Image pushed: $(FULL_IMAGE)${NC}"

test: ## Create test nginx deployment with InfraForge
	@echo "${YELLOW}🧪 Creating test nginx with InfraForge...${NC}"
	@kubectl apply -f claims/test-nginx-v2.yaml
	@echo "${GREEN}✅ Test nginx deployment created${NC}"
	@echo "Run 'make status' to check deployment progress"

status: ## Show InfraForge platform status
	@echo "${BLUE}╦╔╗╔╔═╗╦═╗╔═╗╔═╗╔═╗╦═╗╔═╗╔═╗${NC}"
	@echo "${BLUE}║║║║╠╣ ╠╦╝╠═╣╠╣ ║ ║╠╦╝║ ╦║╣ ${NC}"
	@echo "${BLUE}╩╝╚╝╚  ╩╚═╩ ╩╚  ╚═╝╩╚═╚═╝╚═╝${NC}"
	@echo "${GREEN}Platform Status${NC}"
	@echo ""
	@echo "${YELLOW}📊 Cluster Info${NC}"
	@kubectl cluster-info | head -2
	@echo ""
	@echo "${YELLOW}🔧 Core Components${NC}"
	@kubectl get pods -A | grep -E "(kratix|cert-manager|argocd)" | awk '{printf "%-20s %-40s %-10s\n", $$1, $$2, $$4}'
	@echo ""
	@echo "${YELLOW}📦 InfraForge Resources${NC}"
	@kubectl get infraforges -A 2>/dev/null || echo "No InfraForge resources found"
	@echo ""
	@echo "${YELLOW}⚙️  Kratix Works${NC}"
	@kubectl get works -A 2>/dev/null || echo "No works found"
	@echo ""
	@echo "${YELLOW}🚀 ArgoCD Applications${NC}"
	@kubectl get applications -n $(ARGOCD_NAMESPACE) 2>/dev/null || echo "No applications found"

logs: ## Show InfraForge pipeline logs
	@kubectl logs -n $(KRATIX_NAMESPACE) -l platform.kratix.io/pipeline-name --tail=100 -f

port-forward-argocd: ## Access ArgoCD UI (http://localhost:8080)
	@echo "${YELLOW}Opening ArgoCD UI at http://localhost:8080${NC}"
	@echo "Username: admin"
	@echo "Password: $$(kubectl -n $(ARGOCD_NAMESPACE) get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
	@kubectl port-forward -n $(ARGOCD_NAMESPACE) svc/argocd-server 8080:443

# Development helpers
dev-reset: ## Reset development environment (keeps images)
	@echo "${YELLOW}🔄 Resetting development environment...${NC}"
	@kubectl delete infraforges --all -A 2>/dev/null || true
	@kubectl delete works --all -A 2>/dev/null || true
	@echo "${GREEN}✅ Development environment reset${NC}"