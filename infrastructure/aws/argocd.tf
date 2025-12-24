# ArgoCD locals for configuration
locals {
  argocd_config = {
    argocd_version           = "v2.9.3"
    domain_name              = var.domain_name
    aws_region               = var.aws_region
    oidc_issuer_url          = module.eks.cluster_oidc_issuer_url
    controller_replicas      = 2
    controller_cpu_request   = "250m"
    controller_memory_request = "512Mi"
    controller_cpu_limit     = "500m"
    controller_memory_limit  = "1Gi"
    repo_server_replicas     = 2
    repo_server_cpu_request  = "250m"
    repo_server_memory_request = "256Mi"
    repo_server_cpu_limit    = "500m"
    repo_server_memory_limit = "512Mi"
    redis_cpu_request        = "100m"
    redis_memory_request     = "128Mi"
    redis_cpu_limit          = "200m"
    redis_memory_limit       = "256Mi"
  }
}

# ArgoCD Installation
resource "helm_release" "argocd" {
  name             = "argocd"
  repository       = "https://argoproj.github.io/argo-helm"
  chart            = "argo-cd"
  version          = "5.51.4"
  namespace        = "argocd"
  create_namespace = true

  values = [
    templatefile("${path.module}/templates/argocd-values.yaml.tftpl", local.argocd_config)
  ]

  depends_on = [
    module.eks,
    module.aws_load_balancer_controller_irsa
  ]
}

# ArgoCD Admin Password Secret
resource "random_password" "argocd_admin" {
  length  = 16
  special = true
}

resource "kubernetes_secret" "argocd_admin_password" {
  metadata {
    name      = "argocd-initial-admin-secret"
    namespace = "argocd"
  }

  data = {
    password = bcrypt(random_password.argocd_admin.result)
  }

  depends_on = [helm_release.argocd]
}

# ArgoCD Applications
resource "kubectl_manifest" "argocd_app_of_apps" {
  yaml_body = <<-YAML
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: app-of-apps
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: https://github.com/${var.github_org}/platform-gitops
    path: applications
    targetRevision: main
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
  YAML

  depends_on = [helm_release.argocd]
}