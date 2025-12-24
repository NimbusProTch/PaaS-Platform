# ArgoCD AppProjects - Team � Environment Matrix
# Provides multi-tenancy and RBAC isolation

locals {
  teams = ["ecommerce-team", "analytics-team", "platform-team"]
  environments = ["dev", "qa", "staging", "prod"]

  # Create team � environment combinations
  team_env_combinations = flatten([
    for team in local.teams : [
      for env in local.environments : {
        team = team
        env  = env
        name = "${team}-${env}"
      }
    ]
  ])
}

# Create AppProjects for each team � environment combination
resource "kubectl_manifest" "argocd_appproject" {
  for_each = { for combo in local.team_env_combinations : combo.name => combo }

  yaml_body = yamlencode({
    apiVersion = "argoproj.io/v1alpha1"
    kind       = "AppProject"
    metadata = {
      name      = each.value.name
      namespace = "argocd"
      labels = {
        "platform.infraforge.io/managed" = "true"
        "platform.infraforge.io/team"    = each.value.team
        "platform.infraforge.io/env"     = each.value.env
      }
    }
    spec = {
      description = "Project for ${each.value.team} team in ${each.value.env} environment"

      sourceRepos = [
        "*"  # Allow all repos - can be restricted per team
      ]

      destinations = [
        {
          namespace = each.value.env
          server    = "https://kubernetes.default.svc"
        },
        {
          namespace = "${each.value.team}-${each.value.env}"
          server    = "https://kubernetes.default.svc"
        }
      ]

      clusterResourceWhitelist = [
        {
          group = "*"
          kind  = "*"
        }
      ]

      namespaceResourceWhitelist = [
        {
          group = "*"
          kind  = "*"
        }
      ]

      roles = [
        {
          name = "developer"
          policies = [
            "p, proj:${each.value.name}:developer, applications, get, ${each.value.name}/*, allow",
            "p, proj:${each.value.name}:developer, applications, sync, ${each.value.name}/*, allow",
            "p, proj:${each.value.name}:developer, applications, override, ${each.value.name}/*, allow"
          ]
          groups = [
            "${each.value.team}-developers"
          ]
        },
        {
          name = "admin"
          policies = [
            "p, proj:${each.value.name}:admin, applications, *, ${each.value.name}/*, allow"
          ]
          groups = [
            "${each.value.team}-admins",
            "platform-admins"
          ]
        }
      ]
    }
  })

  depends_on = [
    helm_release.argocd
  ]
}
