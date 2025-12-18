# Backstage Developer Portal Installation

# PostgreSQL for Backstage
resource "helm_release" "postgresql_backstage" {
  name             = "backstage-postgresql"
  repository       = "https://charts.bitnami.com/bitnami"
  chart            = "postgresql"
  version          = "11.9.13"
  namespace        = "backstage"
  create_namespace = true

  values = [
    <<-EOT
    auth:
      database: backstage
      username: backstage
      password: ${random_password.backstage_db_password.result}

    primary:
      persistence:
        enabled: true
        size: 10Gi
        storageClass: gp3

      resources:
        requests:
          memory: 256Mi
          cpu: 250m
        limits:
          memory: 512Mi
          cpu: 500m
    EOT
  ]

  depends_on = [module.eks]
}

resource "random_password" "backstage_db_password" {
  length  = 16
  special = false
}

# Backstage Installation
resource "helm_release" "backstage" {
  name             = "backstage"
  repository       = "https://backstage.github.io/charts"
  chart            = "backstage"
  version          = "1.9.0"
  namespace        = "backstage"
  create_namespace = true

  values = [
    <<-EOT
    backstage:
      image:
        registry: ghcr.io
        repository: backstage/backstage
        tag: latest

      extraEnvVars:
        - name: POSTGRES_HOST
          value: backstage-postgresql
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_USER
          value: backstage
        - name: POSTGRES_PASSWORD
          value: ${random_password.backstage_db_password.result}

      appConfig:
        app:
          title: InfraForge Developer Portal
          baseUrl: https://backstage.${var.domain_name}

        backend:
          baseUrl: https://backstage.${var.domain_name}
          listen:
            port: 7007
          cors:
            origin: https://backstage.${var.domain_name}
          database:
            client: pg
            connection:
              host: backstage-postgresql
              port: 5432
              user: backstage
              password: ${random_password.backstage_db_password.result}
              database: backstage

        integrations:
          github:
            - host: github.com
              token: ${var.github_token}

        catalog:
          import:
            entityFilename: catalog-info.yaml
            pullRequestBranchName: backstage-integration
          locations:
            - type: url
              target: https://github.com/${var.github_org}/software-templates/blob/main/all-templates.yaml

        auth:
          providers:
            github:
              development:
                clientId: ${var.github_oauth_client_id}
                clientSecret: ${var.github_oauth_client_secret}

        techdocs:
          builder: local
          generator:
            runIn: local
          publisher:
            type: local

        kubernetes:
          serviceLocatorMethod:
            type: multiTenant
          clusterLocatorMethods:
            - type: config
              clusters:
                - name: infraforge-dev
                  url: https://kubernetes.default.svc
                  authProvider: serviceAccount
                  serviceAccountToken: ${data.kubernetes_secret.backstage_sa_token.data.token}

        argocd:
          baseUrl: https://argocd.${var.domain_name}
          username: admin
          password: ${random_password.argocd_admin.result}
          appLocatorMethods:
            - type: config
              instances:
                - name: argocd
                  url: https://argocd.${var.domain_name}

        # Kratix Integration
        kratix:
          enabled: true
          apiUrl: https://kubernetes.default.svc

    service:
      type: LoadBalancer
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      ports:
        backend: 7007

    ingress:
      enabled: true
      className: alb
      annotations:
        alb.ingress.kubernetes.io/scheme: internet-facing
        alb.ingress.kubernetes.io/target-type: ip
        alb.ingress.kubernetes.io/certificate-arn: ${var.acm_certificate_arn}
        alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
        alb.ingress.kubernetes.io/ssl-redirect: '443'
      host: backstage.${var.domain_name}

    postgresql:
      enabled: false

    resources:
      requests:
        memory: 512Mi
        cpu: 500m
      limits:
        memory: 1Gi
        cpu: 1000m
    EOT
  ]

  depends_on = [
    helm_release.postgresql_backstage,
    helm_release.argocd,
    kubectl_manifest.kratix_platform
  ]
}

# Service Account for Backstage
resource "kubernetes_service_account" "backstage" {
  metadata {
    name      = "backstage"
    namespace = "backstage"
  }
}

resource "kubernetes_cluster_role" "backstage" {
  metadata {
    name = "backstage-reader"
  }

  rule {
    api_groups = ["", "apps", "batch", "networking.k8s.io"]
    resources  = ["pods", "services", "deployments", "replicasets", "ingresses", "jobs", "cronjobs"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["platform.kratix.io", "marketplace.kratix.io"]
    resources  = ["*"]
    verbs      = ["get", "list", "watch", "create", "update", "patch"]
  }
}

resource "kubernetes_cluster_role_binding" "backstage" {
  metadata {
    name = "backstage-reader"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.backstage.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.backstage.metadata[0].name
    namespace = "backstage"
  }
}

# Get the service account token
data "kubernetes_secret" "backstage_sa_token" {
  metadata {
    name      = kubernetes_service_account.backstage.default_secret_name
    namespace = "backstage"
  }

  depends_on = [kubernetes_service_account.backstage]
}

# Software Templates for Backstage will be added via ArgoCD later

# Output Backstage URL and details
output "backstage_url" {
  value = "https://backstage.${var.domain_name}"
}

output "backstage_db_password" {
  value     = random_password.backstage_db_password.result
  sensitive = true
}