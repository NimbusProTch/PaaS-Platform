# ArgoCD Installation
resource "helm_release" "argocd" {
  name             = "argocd"
  repository       = "https://argoproj.github.io/argo-helm"
  chart            = "argo-cd"
  version          = "5.51.4"
  namespace        = "argocd"
  create_namespace = true

  values = [
    <<-EOT
    global:
      image:
        tag: "v2.9.3"

    server:
      service:
        type: LoadBalancer
        annotations:
          service.beta.kubernetes.io/aws-load-balancer-type: "nlb"

      config:
        url: "https://argocd.${var.domain_name}"

        # OIDC Configuration (optional)
        oidc.config: |
          name: AWS SSO
          issuer: https://oidc.eks.${var.aws_region}.amazonaws.com/id/${module.eks.cluster_oidc_issuer_url}
          clientId: argocd
          requestedScopes: ["openid", "profile", "email"]
          requestedIdTokenClaims: {"groups": {"essential": true}}

      rbacConfig:
        policy.default: role:readonly
        policy.csv: |
          p, role:admin, applications, *, */*, allow
          p, role:admin, clusters, *, *, allow
          p, role:admin, repositories, *, *, allow
          g, argocd-admins, role:admin

    controller:
      replicas: 2
      resources:
        requests:
          cpu: 250m
          memory: 512Mi
        limits:
          cpu: 500m
          memory: 1Gi

    repoServer:
      replicas: 2
      resources:
        requests:
          cpu: 250m
          memory: 256Mi
        limits:
          cpu: 500m
          memory: 512Mi

    redis:
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
        limits:
          cpu: 200m
          memory: 256Mi
    EOT
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

# Output ArgoCD URL and credentials
output "argocd_url" {
  value = "https://argocd.${var.domain_name}"
}

output "argocd_admin_password" {
  value     = random_password.argocd_admin.result
  sensitive = true
}