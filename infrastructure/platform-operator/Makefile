# Platform Operator Makefile

# Image URL to use all building/pushing image targets
IMG ?= platform-operator:latest
# Controller tools
CONTROLLER_TOOLS_VERSION ?= v0.14.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..." paths="./internal/..." paths="./cmd/..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

##@ Build
.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run cmd/manager/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager (must run from project root).
	@echo "Building operator image from project root..."
	@if [ ! -f "../../charts" ]; then \
		echo "Error: Must run 'make docker-build' from project root"; \
		echo "Usage: cd ../.. && make -C infrastructure/platform-operator docker-build"; \
		exit 1; \
	fi
	cd ../.. && docker build -f infrastructure/platform-operator/Dockerfile -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: kind-load
kind-load: docker-build ## Load docker image into kind cluster
	kind load docker-image ${IMG} --name platform-test

##@ Deployment
.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f config/crd/bases

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f config/crd/bases

.PHONY: deploy
deploy: manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && kustomize edit set image controller=${IMG}
	kubectl apply -k config/default

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	kubectl delete -k config/default

.PHONY: deploy-local
deploy-local: docker-build kind-load deploy ## Build, load to kind and deploy

##@ Build Dependencies
## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.14.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest