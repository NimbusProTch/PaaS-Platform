#!/bin/bash

# Enterprise PaaS Platform Deployment Script
# InfraForge Platform Engineering Solution

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PLATFORM_NAME="InfraForge"
PLATFORM_VERSION="1.0.0"
GITHUB_ORG="infraforge"
GITHUB_REPO="gitops"
DOMAIN="infraforge.io"

# Function to print colored output
print_color() {
    color=$1
    message=$2
    echo -e "${color}${message}${NC}"
}

print_header() {
    echo "=================================================="
    print_color "$BLUE" "$1"
    echo "=================================================="
}

print_success() {
    print_color "$GREEN" "✓ $1"
}

print_error() {
    print_color "$RED" "✗ $1"
    exit 1
}

print_warning() {
    print_color "$YELLOW" "⚠ $1"
}

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl is not installed"
    else
        print_success "kubectl found: $(kubectl version --client --short 2>/dev/null)"
    fi

    # Check helm
    if ! command -v helm &> /dev/null; then
        print_error "helm is not installed"
    else
        print_success "helm found: $(helm version --short)"
    fi

    # Check git
    if ! command -v git &> /dev/null; then
        print_error "git is not installed"
    else
        print_success "git found"
    fi

    # Check cluster connection
    if ! kubectl cluster-info &> /dev/null; then
        print_error "Cannot connect to Kubernetes cluster"
    else
        CLUSTER_ENDPOINT=$(kubectl cluster-info | grep "control plane" | awk '{print $NF}')
        print_success "Connected to cluster: $CLUSTER_ENDPOINT"
    fi

    # Check if cluster is EKS
    if kubectl get nodes -o json | jq -r '.items[0].metadata.labels."eks.amazonaws.com/nodegroup"' &> /dev/null; then
        print_success "EKS cluster detected"
        CLUSTER_TYPE="eks"
    else
        print_warning "Non-EKS cluster detected"
        CLUSTER_TYPE="generic"
    fi
}

# Install ArgoCD
install_argocd() {
    print_header "Installing ArgoCD"

    # Create namespace
    kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -

    # Add Helm repo
    helm repo add argo https://argoproj.github.io/argo-helm
    helm repo update

    # Install ArgoCD
    print_color "$BLUE" "Installing ArgoCD..."
    helm upgrade --install argocd argo/argo-cd \
        --namespace argocd \
        --version 5.51.6 \
        --set global.domain=argocd.$DOMAIN \
        --set server.ingress.enabled=true \
        --set server.ingress.ingressClassName=kong \
        --set server.ingress.hosts[0]=argocd.$DOMAIN \
        --set server.ingress.tls[0].secretName=argocd-tls \
        --set server.ingress.tls[0].hosts[0]=argocd.$DOMAIN \
        --set redis-ha.enabled=true \
        --set controller.replicas=1 \
        --set server.replicas=2 \
        --set repoServer.replicas=2 \
        --set applicationSet.enabled=true \
        --set notifications.enabled=true \
        --wait

    print_success "ArgoCD installed successfully"

    # Get initial admin password
    ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
    print_warning "ArgoCD admin password: $ARGOCD_PASSWORD"
    print_warning "Please change this password after first login"
}

# Install Kong Ingress Controller
install_kong() {
    print_header "Installing Kong Ingress Controller"

    kubectl create namespace kong --dry-run=client -o yaml | kubectl apply -f -

    helm repo add kong https://charts.konghq.com
    helm repo update

    helm upgrade --install kong kong/kong \
        --namespace kong \
        --version 2.35.0 \
        --set proxy.type=LoadBalancer \
        --set proxy.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"="nlb" \
        --set ingressController.enabled=true \
        --set ingressController.installCRDs=false \
        --wait

    print_success "Kong installed successfully"
}

# Install Cert Manager
install_cert_manager() {
    print_header "Installing Cert Manager"

    kubectl create namespace cert-manager --dry-run=client -o yaml | kubectl apply -f -

    helm repo add jetstack https://charts.jetstack.io
    helm repo update

    helm upgrade --install cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --version v1.13.3 \
        --set installCRDs=true \
        --wait

    # Create ClusterIssuer for Let's Encrypt
    cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: platform@$DOMAIN
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: kong
EOF

    print_success "Cert Manager installed successfully"
}

# Install Prometheus Stack
install_monitoring() {
    print_header "Installing Monitoring Stack"

    kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update

    helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
        --namespace monitoring \
        --version 55.5.0 \
        --set prometheus.prometheusSpec.retention=30d \
        --set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.storageClassName=gp3 \
        --set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=50Gi \
        --set grafana.ingress.enabled=true \
        --set grafana.ingress.ingressClassName=kong \
        --set grafana.ingress.hosts[0]=grafana.$DOMAIN \
        --set grafana.persistence.enabled=true \
        --set grafana.persistence.storageClassName=gp3 \
        --set grafana.persistence.size=10Gi \
        --wait

    print_success "Monitoring stack installed successfully"
}

# Install Kratix
install_kratix() {
    print_header "Installing Kratix Platform"

    kubectl create namespace kratix-platform-system --dry-run=client -o yaml | kubectl apply -f -

    # Install Kratix
    kubectl apply -f https://github.com/syntasso/kratix/releases/latest/download/kratix.yaml

    # Wait for Kratix to be ready
    kubectl wait --for=condition=Ready pods -l app.kubernetes.io/name=kratix -n kratix-platform-system --timeout=300s

    print_success "Kratix installed successfully"
}

# Install Backstage
install_backstage() {
    print_header "Installing Backstage Developer Portal"

    kubectl create namespace backstage --dry-run=client -o yaml | kubectl apply -f -

    # Create PostgreSQL for Backstage
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
  namespace: backstage
type: Opaque
stringData:
  POSTGRES_USER: backstage
  POSTGRES_PASSWORD: $(openssl rand -base64 32)
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: backstage
spec:
  serviceName: postgres
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15
        envFrom:
        - secretRef:
            name: postgres-secret
        env:
        - name: POSTGRES_DB
          value: backstage
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: postgres-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: gp3
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: backstage
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
EOF

    print_success "Backstage prerequisites installed"
    print_warning "Backstage deployment requires custom configuration"
}

# Setup GitOps repository
setup_gitops() {
    print_header "Setting up GitOps Repository"

    # Apply app-of-apps pattern
    cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-apps
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: https://github.com/$GITHUB_ORG/$GITHUB_REPO
    targetRevision: HEAD
    path: gitops/platform
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
      allowEmpty: false
    syncOptions:
    - CreateNamespace=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
EOF

    print_success "GitOps repository configured"
}

# Create sample tenant
create_sample_tenant() {
    print_header "Creating Sample Tenant"

    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: team-alpha-dev
  labels:
    tenant: team-alpha
    environment: development
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: team-alpha-developer
  namespace: team-alpha-dev
rules:
- apiGroups: ["", "apps", "batch", "networking.k8s.io"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-alpha-developers
  namespace: team-alpha-dev
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: team-alpha-developer
subjects:
- kind: Group
  name: team-alpha
  apiGroup: rbac.authorization.k8s.io
EOF

    print_success "Sample tenant created"
}

# Main deployment function
deploy_platform() {
    print_header "Starting $PLATFORM_NAME Platform Deployment"
    print_color "$BLUE" "Version: $PLATFORM_VERSION"
    print_color "$BLUE" "Domain: $DOMAIN"
    echo ""

    check_prerequisites

    # Core components
    install_kong
    install_cert_manager
    install_argocd

    # Platform components
    install_monitoring
    install_kratix
    install_backstage

    # GitOps setup
    setup_gitops

    # Sample configuration
    create_sample_tenant

    print_header "Platform Deployment Complete!"

    echo ""
    print_color "$GREEN" "Platform URLs:"
    echo "  ArgoCD:    https://argocd.$DOMAIN"
    echo "  Grafana:   https://grafana.$DOMAIN"
    echo "  Backstage: https://backstage.$DOMAIN"

    echo ""
    print_color "$YELLOW" "Next Steps:"
    echo "1. Update DNS records to point to the Kong LoadBalancer"
    echo "2. Configure Backstage with your GitHub organization"
    echo "3. Import service templates into Backstage"
    echo "4. Configure SSO with your identity provider"
    echo "5. Create Kratix promises for your platform services"

    echo ""
    print_color "$BLUE" "To get Kong LoadBalancer address:"
    echo "kubectl get svc -n kong kong-proxy -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'"
}

# Script execution
case "${1:-}" in
    "")
        deploy_platform
        ;;
    "--help"|"-h")
        echo "Usage: $0 [OPTION]"
        echo "Deploy the $PLATFORM_NAME Enterprise PaaS Platform"
        echo ""
        echo "Options:"
        echo "  --help, -h     Show this help message"
        echo "  --uninstall    Remove all platform components"
        echo "  --status       Check platform status"
        ;;
    "--uninstall")
        print_header "Uninstalling Platform"
        kubectl delete namespace argocd backstage kong cert-manager monitoring kratix-platform-system team-alpha-dev --ignore-not-found
        print_success "Platform uninstalled"
        ;;
    "--status")
        print_header "Platform Status"
        kubectl get pods -n argocd
        kubectl get pods -n kong
        kubectl get pods -n monitoring
        kubectl get pods -n backstage
        kubectl get pods -n kratix-platform-system
        ;;
    *)
        print_error "Unknown option: $1"
        ;;
esac