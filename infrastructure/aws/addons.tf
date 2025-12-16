# AWS EKS Addons and Core Components

# EBS CSI Driver - Already in main EKS module
# But we need to create storage classes

# Storage Classes
resource "kubectl_manifest" "gp3_storage_class" {
  yaml_body = <<YAML
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Delete
YAML

  depends_on = [module.eks]
}

resource "kubectl_manifest" "gp3_retain_storage_class" {
  yaml_body = <<YAML
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3-retain
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Retain
YAML

  depends_on = [module.eks]
}

# Metrics Server
resource "helm_release" "metrics_server" {
  name       = "metrics-server"
  repository = "https://kubernetes-sigs.github.io/metrics-server/"
  chart      = "metrics-server"
  namespace  = "kube-system"
  version    = "3.12.0"

  values = [<<EOF
args:
  - --cert-dir=/tmp
  - --kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname
  - --kubelet-use-node-status-port
  - --metric-resolution=15s
  - --kubelet-insecure-tls

resources:
  requests:
    cpu: 100m
    memory: 200Mi
  limits:
    cpu: 500m
    memory: 512Mi

podDisruptionBudget:
  enabled: true
  minAvailable: 1

nodeSelector:
  kubernetes.io/os: linux

tolerations:
  - key: "CriticalAddonsOnly"
    operator: "Exists"
EOF
  ]

  depends_on = [module.eks]
}

# Cluster Autoscaler
resource "helm_release" "cluster_autoscaler" {
  count = var.enable_cluster_autoscaler ? 1 : 0

  name       = "cluster-autoscaler"
  repository = "https://kubernetes.github.io/autoscaler"
  chart      = "cluster-autoscaler"
  namespace  = "kube-system"
  version    = "9.35.0"

  values = [<<EOF
autoDiscovery:
  clusterName: ${module.eks.cluster_name}
  enabled: true

awsRegion: ${var.aws_region}

rbac:
  serviceAccount:
    create: true
    name: cluster-autoscaler
    annotations:
      eks.amazonaws.com/role-arn: ${var.enable_cluster_autoscaler ? module.cluster_autoscaler_irsa[0].iam_role_arn : ""}

resources:
  limits:
    cpu: 100m
    memory: 300Mi
  requests:
    cpu: 100m
    memory: 300Mi

nodeSelector:
  kubernetes.io/os: linux

image:
  tag: v1.28.2

extraArgs:
  skip-nodes-with-local-storage: false
  skip-nodes-with-system-pods: false
  balance-similar-node-groups: true
  expander: least-waste
EOF
  ]

  depends_on = [
    module.eks,
    module.cluster_autoscaler_irsa
  ]
}

# Kong Ingress Controller with Gateway API support
resource "helm_release" "kong" {
  name       = "kong"
  repository = "https://charts.konghq.com"
  chart      = "kong"
  namespace  = "kong"
  version    = "2.35.0"

  create_namespace = true

  values = [<<EOF
image:
  repository: kong/kong-gateway
  tag: "3.5"

env:
  database: "off"
  nginx_worker_processes: "2"
  proxy_access_log: /dev/stdout
  admin_access_log: /dev/stdout
  admin_gui_access_log: /dev/stdout
  portal_api_access_log: /dev/stdout
  proxy_error_log: /dev/stderr
  admin_error_log: /dev/stderr
  admin_gui_error_log: /dev/stderr
  portal_api_error_log: /dev/stderr
  prefix: /kong_prefix/
  log_level: info

proxy:
  enabled: true
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"

  http:
    enabled: true
    servicePort: 80
    containerPort: 8000

  tls:
    enabled: true
    servicePort: 443
    containerPort: 8443

  ingress:
    enabled: false

admin:
  enabled: true
  type: ClusterIP
  http:
    enabled: true
    servicePort: 8001
    containerPort: 8001

manager:
  enabled: true
  type: ClusterIP

ingressController:
  enabled: true
  image:
    repository: kong/kubernetes-ingress-controller
    tag: "3.0"

  env:
    kong_admin_tls_skip_verify: true
    publish_service: kong/kong-proxy

  # Enable Gateway API support
  gatewayAPI:
    enabled: true

  resources:
    limits:
      cpu: 200m
      memory: 256Mi
    requests:
      cpu: 100m
      memory: 128Mi

# Enable Gateway API CRDs
gateway:
  enabled: true

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80

resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi

nodeSelector:
  kubernetes.io/os: linux

tolerations: []

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - kong
        topologyKey: kubernetes.io/hostname
EOF
  ]

  depends_on = [module.eks]
}

# Gateway API CRDs
resource "kubectl_manifest" "gateway_api_crds" {
  yaml_body = file("https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml")

  depends_on = [module.eks]
}

# AWS Load Balancer Controller
resource "helm_release" "aws_load_balancer_controller" {
  count = var.enable_aws_load_balancer_controller ? 1 : 0

  name       = "aws-load-balancer-controller"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-load-balancer-controller"
  namespace  = "kube-system"
  version    = "1.6.2"

  values = [<<EOF
clusterName: ${module.eks.cluster_name}
region: ${var.aws_region}
vpcId: ${module.vpc.vpc_id}

serviceAccount:
  create: true
  name: aws-load-balancer-controller
  annotations:
    eks.amazonaws.com/role-arn: ${var.enable_aws_load_balancer_controller ? module.aws_load_balancer_controller_irsa[0].iam_role_arn : ""}

defaultTags:
  Environment: ${var.environment}
  ManagedBy: Terraform

resources:
  limits:
    cpu: 200m
    memory: 500Mi
  requests:
    cpu: 100m
    memory: 200Mi

nodeSelector:
  kubernetes.io/os: linux

tolerations:
  - key: "CriticalAddonsOnly"
    operator: "Exists"
EOF
  ]

  depends_on = [
    module.eks,
    module.aws_load_balancer_controller_irsa
  ]
}

# External DNS for automatic Route53 DNS management
resource "helm_release" "external_dns" {
  count = var.enable_external_dns ? 1 : 0

  name       = "external-dns"
  repository = "https://kubernetes-sigs.github.io/external-dns/"
  chart      = "external-dns"
  namespace  = "external-dns"
  version    = "1.14.3"

  create_namespace = true

  values = [<<EOF
provider: aws
aws:
  region: ${var.aws_region}
  zoneType: public

domainFilters:
  - ${var.domain_name}

policy: sync
registry: txt
txtOwnerId: ${module.eks.cluster_name}

serviceAccount:
  create: true
  name: external-dns
  annotations:
    eks.amazonaws.com/role-arn: ${var.enable_external_dns ? module.external_dns_irsa[0].iam_role_arn : ""}

resources:
  limits:
    cpu: 100m
    memory: 300Mi
  requests:
    cpu: 50m
    memory: 100Mi

nodeSelector:
  kubernetes.io/os: linux
EOF
  ]

  depends_on = [
    module.eks,
    module.external_dns_irsa
  ]
}

# Cert Manager for automatic TLS certificate management
resource "helm_release" "cert_manager" {
  count = var.enable_cert_manager ? 1 : 0

  name       = "cert-manager"
  repository = "https://charts.jetstack.io"
  chart      = "cert-manager"
  namespace  = "cert-manager"
  version    = "v1.13.3"

  create_namespace = true

  values = [<<EOF
installCRDs: true

global:
  leaderElection:
    namespace: cert-manager

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 200m
    memory: 256Mi

webhook:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 100m
      memory: 128Mi

cainjector:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 256Mi

nodeSelector:
  kubernetes.io/os: linux

prometheus:
  enabled: true
EOF
  ]

  depends_on = [module.eks]
}

# Karpenter for advanced node autoscaling (alternative to Cluster Autoscaler)
resource "helm_release" "karpenter" {
  count = var.enable_karpenter ? 1 : 0

  name       = "karpenter"
  repository = "oci://public.ecr.aws/karpenter"
  chart      = "karpenter"
  namespace  = "karpenter"
  version    = "v0.33.0"

  create_namespace = true

  values = [<<EOF
settings:
  aws:
    clusterName: ${module.eks.cluster_name}
    defaultInstanceProfile: ${aws_iam_instance_profile.karpenter[0].name}
    interruptionQueueName: ${module.eks.cluster_name}
    vmMemoryOverheadPercent: 0.075

serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: ${var.enable_karpenter ? aws_iam_role.karpenter[0].arn : ""}

controller:
  resources:
    requests:
      cpu: 1
      memory: 1Gi
    limits:
      cpu: 1
      memory: 1Gi

webhook:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
EOF
  ]

  depends_on = [module.eks]
}

# VPA (Vertical Pod Autoscaler) for resource optimization
resource "helm_release" "vpa" {
  count = var.enable_vpa ? 1 : 0

  name       = "vpa"
  repository = "https://charts.fairwinds.com/stable"
  chart      = "vpa"
  namespace  = "vpa-system"
  version    = "3.0.0"

  create_namespace = true

  values = [<<EOF
recommender:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 512Mi

updater:
  enabled: true
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 512Mi

admissionController:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 512Mi
EOF
  ]

  depends_on = [module.eks]
}