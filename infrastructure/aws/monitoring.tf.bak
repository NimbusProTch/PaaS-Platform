# Monitoring Stack - Prometheus, Grafana, Loki, Tempo

# Prometheus Stack (includes Prometheus, Grafana, AlertManager)
resource "helm_release" "kube_prometheus_stack" {
  count = var.enable_prometheus ? 1 : 0

  name       = "kube-prometheus-stack"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  namespace  = "monitoring"
  version    = "55.5.0"

  create_namespace = true

  values = [<<EOF
fullnameOverride: prometheus

prometheus:
  prometheusSpec:
    retention: 30d
    retentionSize: "50GB"

    storageSpec:
      volumeClaimTemplate:
        spec:
          storageClassName: gp3
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 50Gi

    resources:
      requests:
        cpu: 500m
        memory: 2Gi
      limits:
        cpu: 2000m
        memory: 4Gi

    # Service monitors
    serviceMonitorSelectorNilUsesHelmValues: false
    podMonitorSelectorNilUsesHelmValues: false
    ruleSelectorNilUsesHelmValues: false

  ingress:
    enabled: false

alertmanager:
  alertmanagerSpec:
    retention: 120h
    storage:
      volumeClaimTemplate:
        spec:
          storageClassName: gp3
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 10Gi

    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi

grafana:
  enabled: ${var.enable_grafana}

  adminPassword: ${random_password.grafana_admin_password.result}

  persistence:
    enabled: true
    storageClassName: gp3
    size: 10Gi

  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

  # Kong and API Gateway dashboards
  dashboardProviders:
    dashboardproviders.yaml:
      apiVersion: 1
      providers:
      - name: 'default'
        orgId: 1
        folder: ''
        type: file
        disableDeletion: false
        editable: true
        options:
          path: /var/lib/grafana/dashboards/default
      - name: 'kong'
        orgId: 1
        folder: 'Kong'
        type: file
        disableDeletion: false
        editable: true
        options:
          path: /var/lib/grafana/dashboards/kong

  dashboards:
    default:
      kubernetes-cluster:
        gnetId: 7249
        revision: 1
        datasource: Prometheus
      kubernetes-pods:
        gnetId: 6417
        revision: 1
        datasource: Prometheus
      node-exporter:
        gnetId: 11074
        revision: 1
        datasource: Prometheus
    kong:
      kong-dashboard:
        gnetId: 7424
        revision: 6
        datasource: Prometheus
      kong-gateway-api:
        gnetId: 13115
        revision: 1
        datasource: Prometheus

  # Data sources
  sidecar:
    datasources:
      enabled: true
      defaultDatasourceEnabled: true

  # Plugins
  plugins:
    - redis-app
    - grafana-piechart-panel
    - grafana-kubernetes-app

  ingress:
    enabled: false

# Node exporter for system metrics
prometheus-node-exporter:
  resources:
    requests:
      cpu: 50m
      memory: 32Mi
    limits:
      cpu: 200m
      memory: 64Mi

# Kube state metrics
kube-state-metrics:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi

# Operator
prometheusOperator:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

# Service monitors for operators
defaultRules:
  create: true
  rules:
    alertmanager: true
    etcd: true
    configReloaders: true
    general: true
    k8s: true
    kubeApiserverAvailability: true
    kubeApiserverBurnrate: true
    kubeApiserverHistogram: true
    kubeApiserverSlos: true
    kubelet: true
    kubePrometheusGeneral: true
    kubePrometheusNodeRecording: true
    kubernetesApps: true
    kubernetesResources: true
    kubernetesStorage: true
    kubernetesSystem: true
    kubeScheduler: true
    kubeStateMetrics: true
    network: true
    node: true
    nodeExporterAlerting: true
    nodeExporterRecording: true
    prometheus: true
    prometheusOperator: true
EOF
  ]

  depends_on = [
    module.eks,
    kubectl_manifest.gp3_storage_class
  ]
}

# Random password for Grafana admin
resource "random_password" "grafana_admin_password" {
  length  = 16
  special = true
}

# Store Grafana password in AWS Secrets Manager
resource "aws_secretsmanager_secret" "grafana_admin_password" {
  count = var.enable_grafana ? 1 : 0

  name = "${local.cluster_name}-grafana-admin-password"
  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "grafana_admin_password" {
  count = var.enable_grafana ? 1 : 0

  secret_id     = aws_secretsmanager_secret.grafana_admin_password[0].id
  secret_string = random_password.grafana_admin_password.result
}

# Loki for log aggregation
resource "helm_release" "loki" {
  count = var.enable_loki ? 1 : 0

  name       = "loki"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "loki"
  namespace  = "monitoring"
  version    = "5.41.0"

  values = [<<EOF
loki:
  auth_enabled: false

  storage:
    type: s3
    s3:
      endpoint: s3.${var.aws_region}.amazonaws.com
      region: ${var.aws_region}
      bucketnames: ${aws_s3_bucket.loki[0].id}
      insecure: false
      s3forcepathstyle: false

  schema_config:
    configs:
    - from: 2024-01-01
      store: boltdb-shipper
      object_store: s3
      schema: v12
      index:
        prefix: loki_index_
        period: 24h

  limits_config:
    enforce_metric_name: false
    reject_old_samples: true
    reject_old_samples_max_age: 168h
    max_cache_freshness_per_query: 10m

singleBinary:
  replicas: 1

  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi

  persistence:
    enabled: true
    size: 10Gi
    storageClass: gp3

serviceAccount:
  create: true
  annotations:
    eks.amazonaws.com/role-arn: ${var.enable_loki ? aws_iam_role.loki[0].arn : ""}

monitoring:
  dashboards:
    enabled: true
  rules:
    enabled: true
  serviceMonitor:
    enabled: true
EOF
  ]

  depends_on = [
    module.eks,
    helm_release.kube_prometheus_stack
  ]
}

# S3 bucket for Loki logs
resource "aws_s3_bucket" "loki" {
  count = var.enable_loki ? 1 : 0

  bucket = "${local.cluster_name}-loki-logs"
  tags   = local.common_tags
}

# IAM role for Loki
resource "aws_iam_role" "loki" {
  count = var.enable_loki ? 1 : 0

  name = "${local.cluster_name}-loki"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRoleWithWebIdentity"
        Effect = "Allow"
        Principal = {
          Federated = module.eks.oidc_provider_arn
        }
        Condition = {
          StringEquals = {
            "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:sub" = "system:serviceaccount:monitoring:loki"
          }
        }
      }
    ]
  })

  tags = local.common_tags
}

# IAM policy for Loki S3 access
resource "aws_iam_role_policy" "loki_s3" {
  count = var.enable_loki ? 1 : 0

  name = "${local.cluster_name}-loki-s3"
  role = aws_iam_role.loki[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:ListBucket",
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject"
        ]
        Resource = [
          aws_s3_bucket.loki[0].arn,
          "${aws_s3_bucket.loki[0].arn}/*"
        ]
      }
    ]
  })
}

# Promtail for log collection
resource "helm_release" "promtail" {
  count = var.enable_loki ? 1 : 0

  name       = "promtail"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "promtail"
  namespace  = "monitoring"
  version    = "6.15.3"

  values = [<<EOF
config:
  clients:
    - url: http://loki:3100/loki/api/v1/push

resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 200m
    memory: 128Mi

tolerations:
  - effect: NoSchedule
    key: node-role.kubernetes.io/master
    operator: Exists
  - effect: NoSchedule
    key: node-role.kubernetes.io/control-plane
    operator: Exists

serviceMonitor:
  enabled: true
EOF
  ]

  depends_on = [
    helm_release.loki
  ]
}

# Tempo for distributed tracing
resource "helm_release" "tempo" {
  count = var.enable_tempo ? 1 : 0

  name       = "tempo"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "tempo"
  namespace  = "monitoring"
  version    = "1.7.1"

  values = [<<EOF
tempo:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

  storage:
    trace:
      backend: s3
      s3:
        bucket: ${var.enable_tempo ? aws_s3_bucket.tempo[0].id : ""}
        endpoint: s3.${var.aws_region}.amazonaws.com
        region: ${var.aws_region}

serviceAccount:
  create: true
  annotations:
    eks.amazonaws.com/role-arn: ${var.enable_tempo ? aws_iam_role.tempo[0].arn : ""}

persistence:
  enabled: true
  storageClass: gp3
  size: 10Gi
EOF
  ]

  depends_on = [
    module.eks,
    kubectl_manifest.gp3_storage_class
  ]
}

# S3 bucket for Tempo traces
resource "aws_s3_bucket" "tempo" {
  count = var.enable_tempo ? 1 : 0

  bucket = "${local.cluster_name}-tempo-traces"
  tags   = local.common_tags
}

# IAM role for Tempo
resource "aws_iam_role" "tempo" {
  count = var.enable_tempo ? 1 : 0

  name = "${local.cluster_name}-tempo"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRoleWithWebIdentity"
        Effect = "Allow"
        Principal = {
          Federated = module.eks.oidc_provider_arn
        }
        Condition = {
          StringEquals = {
            "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:sub" = "system:serviceaccount:monitoring:tempo"
          }
        }
      }
    ]
  })

  tags = local.common_tags
}

# IAM policy for Tempo S3 access
resource "aws_iam_role_policy" "tempo_s3" {
  count = var.enable_tempo ? 1 : 0

  name = "${local.cluster_name}-tempo-s3"
  role = aws_iam_role.tempo[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:ListBucket",
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject"
        ]
        Resource = [
          aws_s3_bucket.tempo[0].arn,
          "${aws_s3_bucket.tempo[0].arn}/*"
        ]
      }
    ]
  })
}