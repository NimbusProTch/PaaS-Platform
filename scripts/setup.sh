#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CLUSTER_NAME="paas-platform"
TIMEOUT=300

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
    exit 1
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

check_requirements() {
    log "Checking requirements..."
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed"
    fi
    
    # Check Kind
    if ! command -v kind &> /dev/null; then
        error "Kind is not installed. Install from: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    fi
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        error "kubectl is not installed"
    fi
    
    # Check helm
    if ! command -v helm &> /dev/null; then
        warning "Helm is not installed. Some features may not work."
    fi
    
    log "All requirements met ✓"
}

clean_environment() {
    log "Cleaning environment..."
    
    # Delete Kind cluster
    if kind get clusters | grep -q "$CLUSTER_NAME"; then
        kind delete cluster --name "$CLUSTER_NAME"
    fi
    
    # Clean Docker
    docker rm -f $(docker ps -aq --filter label=io.x-k8s.kind.cluster="$CLUSTER_NAME") 2>/dev/null || true
    
    # Optional: Clean unused Docker resources
    read -p "Clean all unused Docker resources? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker system prune -a --volumes -f
    fi
}

create_cluster() {
    log "Creating Kind cluster..."
    
    # Create Kind config
    cat > /tmp/kind-config.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${CLUSTER_NAME}
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "environment=platform"
  extraMounts:
  - hostPath: /tmp
    containerPath: /tmp
EOF

    kind create cluster --config /tmp/kind-config.yaml --wait 5m
    
    # Verify cluster
    kubectl cluster-info --context kind-${CLUSTER_NAME}
    log "Cluster created successfully ✓"
}

install_cert_manager() {
    log "Installing cert-manager..."
    
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
    
    # Wait for cert-manager
    kubectl wait --for=condition=available --timeout=${TIMEOUT}s \
        -n cert-manager deployment/cert-manager \
        deployment/cert-manager-webhook \
        deployment/cert-manager-cainjector
    
    log "cert-manager installed ✓"
}

install_kratix() {
    log "Installing Kratix..."
    
    kubectl apply -f https://github.com/syntasso/kratix/releases/latest/download/kratix.yaml
    
    # Wait for Kratix
    kubectl wait --for=condition=available --timeout=${TIMEOUT}s \
        -n kratix-platform-system deployment/kratix-platform-controller-manager
    
    log "Kratix installed ✓"
}

install_mongodb_operator() {
    log "Installing MongoDB Community Operator..."
    
    kubectl apply -f https://github.com/mongodb/mongodb-kubernetes-operator/releases/download/v0.8.3/deploy/k8s/mongodb-kubernetes-operator.yaml
    
    # Wait for operator
    kubectl wait --for=condition=available --timeout=${TIMEOUT}s \
        -n mongodb deployment/mongodb-kubernetes-operator
    
    log "MongoDB operator installed ✓"
}

install_gitea() {
    log "Installing Gitea..."
    
    kubectl create namespace gitea || true
    
    # Add Helm repo
    helm repo add gitea-charts https://gitea.com/gitea/helm-chart || true
    helm repo update
    
    # Install Gitea
    helm upgrade --install gitea gitea-charts/gitea \
        --namespace gitea \
        --set persistence.enabled=true \
        --set persistence.size=1Gi \
        --set gitea.admin.username=admin \
        --set gitea.admin.password=admin123 \
        --set service.http.type=ClusterIP \
        --wait --timeout ${TIMEOUT}s
    
    # Create repository
    sleep 10
    kubectl exec -n gitea deploy/gitea -- su git -c "
        gitea admin repo create --name platform-manifests --owner admin || true
    "
    
    log "Gitea installed ✓"
}

install_argocd() {
    log "Installing ArgoCD..."
    
    kubectl create namespace argocd || true
    kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
    
    # Wait for ArgoCD
    kubectl wait --for=condition=available --timeout=${TIMEOUT}s \
        -n argocd deployment --all
    
    # Get admin password
    ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
    log "ArgoCD admin password: $ARGOCD_PASSWORD"
    
    log "ArgoCD installed ✓"
}

configure_platform() {
    log "Configuring platform..."
    
    # Apply GitStateStore
    kubectl apply -f infrastructure/kratix/git-state-store.yaml
    
    # Apply Destination
    kubectl apply -f infrastructure/kratix/destination.yaml
    
    # Apply Promise
    kubectl apply -f infrastructure/kratix/platform-promise.yaml
    
    log "Platform configured ✓"
}

show_status() {
    log "Platform Status:"
    echo
    echo "=== Cluster Info ==="
    kubectl cluster-info
    echo
    echo "=== System Pods ==="
    kubectl get pods -n cert-manager
    kubectl get pods -n kratix-platform-system
    kubectl get pods -n mongodb
    kubectl get pods -n gitea
    kubectl get pods -n argocd
    echo
    echo "=== Platform Resources ==="
    kubectl get promises -A
    kubectl get destinations -A
    kubectl get gitstatestores -A
    echo
    echo "=== Access Info ==="
    echo "Gitea: kubectl port-forward -n gitea svc/gitea-http 3000:3000"
    echo "       Username: admin, Password: admin123"
    echo "ArgoCD: kubectl port-forward -n argocd svc/argocd-server 8080:443"
    echo "        Username: admin, Password: (see above)"
}

# Main execution
main() {
    echo -e "${GREEN}PaaS Platform Setup${NC}"
    echo "===================="
    
    check_requirements
    
    # Ask for clean install
    read -p "Perform clean installation? This will delete existing cluster (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        clean_environment
    fi
    
    create_cluster
    install_cert_manager
    install_kratix
    install_mongodb_operator
    install_gitea
    install_argocd
    configure_platform
    
    echo
    log "🎉 Setup complete!"
    echo
    show_status
}

# Run main function
main "$@"