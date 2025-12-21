# ChartMuseum - Helm Chart Repository
# Stores the common chart for microservices and platform components

resource "kubernetes_namespace" "chartmuseum" {
  metadata {
    name = "chartmuseum"

    labels = {
      "platform.infraforge.io/managed" = "true"
      "platform.infraforge.io/component" = "chartmuseum"
    }
  }

  depends_on = [module.eks]
}

resource "helm_release" "chartmuseum" {
  name       = "chartmuseum"
  repository = "https://chartmuseum.github.io/charts"
  chart      = "chartmuseum"
  version    = "3.10.1"
  namespace  = kubernetes_namespace.chartmuseum.metadata[0].name

  values = [
    yamlencode({
      env = {
        open = {
          DISABLE_API = false
          STORAGE     = "local"
        }
      }

      persistence = {
        enabled      = true
        size         = "10Gi"
        storageClass = "gp3"
      }

      service = {
        type = "ClusterIP"
        port = 8080
      }

      resources = {
        requests = {
          cpu    = "100m"
          memory = "128Mi"
        }
        limits = {
          cpu    = "500m"
          memory = "512Mi"
        }
      }
    })
  ]

  depends_on = [
    kubernetes_namespace.chartmuseum,
    helm_release.argocd
  ]
}

output "chartmuseum_url" {
  description = "ChartMuseum internal URL"
  value       = "http://chartmuseum.chartmuseum.svc:8080"
}
