# ArgoCD Root Apps Configuration

# ArgoCD Root App for NonProd
resource "kubectl_manifest" "argocd_root_app_nonprod" {
  yaml_body = <<-YAML
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app-nonprod
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: ${var.gitops_repo_url}
    targetRevision: ${var.gitops_repo_branch}
    path: environments/nonprod
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
      - ApplyOutOfSyncOnly=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
YAML

  depends_on = [helm_release.argocd]
}

# ArgoCD Root App for Prod
resource "kubectl_manifest" "argocd_root_app_prod" {
  yaml_body = <<-YAML
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app-prod
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: ${var.gitops_repo_url}
    targetRevision: ${var.gitops_repo_branch}
    path: environments/prod
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: false  # Manual sync for production
      selfHeal: false
      allowEmpty: false
    syncOptions:
      - CreateNamespace=true
      - ApplyOutOfSyncOnly=true
    retry:
      limit: 3
      backoff:
        duration: 10s
        factor: 2
        maxDuration: 5m
YAML

  depends_on = [helm_release.argocd]
}

# ArgoCD Projects
resource "kubectl_manifest" "argocd_project_platform" {
  yaml_body = <<-YAML
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: platform
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  description: Platform services project
  sourceRepos:
    - '*'
  destinations:
    - namespace: '*'
      server: https://kubernetes.default.svc
  clusterResourceWhitelist:
    - group: '*'
      kind: '*'
  namespaceResourceWhitelist:
    - group: '*'
      kind: '*'
YAML

  depends_on = [helm_release.argocd]
}

resource "kubectl_manifest" "argocd_project_business" {
  yaml_body = <<-YAML
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: business
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  description: Business applications project
  sourceRepos:
    - '*'
  destinations:
    - namespace: '*'
      server: https://kubernetes.default.svc
  clusterResourceWhitelist:
    - group: '*'
      kind: '*'
  namespaceResourceWhitelist:
    - group: '*'
      kind: '*'
YAML

  depends_on = [helm_release.argocd]
}

# Create namespaces for environments
resource "kubernetes_namespace" "environments" {
  for_each = toset(["dev", "qa", "sandbox", "staging", "prod"])

  metadata {
    name = each.key
    labels = {
      environment = each.key
      managed-by  = "terraform"
    }
  }

  depends_on = [module.eks]
}

# RBAC for ArgoCD to manage resources
resource "kubectl_manifest" "argocd_cluster_role" {
  yaml_body = <<-YAML
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: argocd-manager
rules:
  - apiGroups:
      - "*"
    resources:
      - "*"
    verbs:
      - "*"
  - nonResourceURLs:
      - "*"
    verbs:
      - "*"
YAML

  depends_on = [helm_release.argocd]
}

resource "kubectl_manifest" "argocd_cluster_role_binding" {
  yaml_body = <<-YAML
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: argocd-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: argocd-manager
subjects:
  - kind: ServiceAccount
    name: argocd-application-controller
    namespace: argocd
  - kind: ServiceAccount
    name: argocd-server
    namespace: argocd
YAML

  depends_on = [
    helm_release.argocd,
    kubectl_manifest.argocd_cluster_role
  ]
}