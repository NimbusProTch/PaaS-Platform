# EKS Add-ons and Essential Components

# Locals for addon configurations
locals {
  metrics_server_config = {
    cpu_request    = "100m"
    memory_request = "200Mi"
    cpu_limit      = "200m"
    memory_limit   = "400Mi"
  }

  cluster_autoscaler_config = {
    cluster_name   = local.cluster_name
    aws_region     = var.aws_region
    irsa_role_arn  = module.cluster_autoscaler_irsa[0].iam_role_arn
    cpu_request    = "100m"
    memory_request = "300Mi"
    cpu_limit      = "200m"
    memory_limit   = "500Mi"
  }

  aws_lb_controller_config = {
    cluster_name   = local.cluster_name
    aws_region     = var.aws_region
    vpc_id         = module.vpc.vpc_id
    irsa_role_arn  = module.aws_load_balancer_controller_irsa[0].iam_role_arn
    cpu_request    = "100m"
    memory_request = "128Mi"
    cpu_limit      = "200m"
    memory_limit   = "256Mi"
  }

  external_dns_config = {
    aws_region     = var.aws_region
    domain_name    = var.domain_name
    irsa_role_arn  = module.external_dns_irsa[0].iam_role_arn
    cpu_request    = "50m"
    memory_request = "100Mi"
    cpu_limit      = "100m"
    memory_limit   = "200Mi"
  }

  karpenter_config = {
    cluster_name         = local.cluster_name
    cluster_endpoint     = module.eks.cluster_endpoint
    vm_memory_overhead   = "0.075"
    replicas             = 2
    cpu_request          = "1"
    memory_request       = "1Gi"
    cpu_limit            = "2"
    memory_limit         = "2Gi"
  }

  vpa_config = {
    recommender_cpu_request    = "50m"
    recommender_memory_request = "100Mi"
    recommender_cpu_limit      = "100m"
    recommender_memory_limit   = "200Mi"
    updater_enabled            = "false"
    admission_cpu_request      = "50m"
    admission_memory_request   = "100Mi"
    admission_cpu_limit        = "100m"
    admission_memory_limit     = "200Mi"
  }
}

# Metrics Server
resource "helm_release" "metrics_server" {
  name             = "metrics-server"
  repository       = "https://kubernetes-sigs.github.io/metrics-server/"
  chart            = "metrics-server"
  version          = "3.12.0"
  namespace        = "kube-system"

  values = [
    templatefile("${path.module}/templates/metrics-server-values.yaml.tftpl", local.metrics_server_config)
  ]

  depends_on = [module.eks]
}

# Cluster Autoscaler
resource "helm_release" "cluster_autoscaler" {
  name             = "cluster-autoscaler"
  repository       = "https://kubernetes.github.io/autoscaler"
  chart            = "cluster-autoscaler"
  version          = "9.34.0"
  namespace        = "kube-system"

  values = [
    templatefile("${path.module}/templates/cluster-autoscaler-values.yaml.tftpl", local.cluster_autoscaler_config)
  ]

  depends_on = [
    module.eks,
    module.cluster_autoscaler_irsa
  ]
}

# AWS Load Balancer Controller
resource "helm_release" "aws_load_balancer_controller" {
  name             = "aws-load-balancer-controller"
  repository       = "https://aws.github.io/eks-charts"
  chart            = "aws-load-balancer-controller"
  version          = "1.6.2"
  namespace        = "kube-system"

  values = [
    templatefile("${path.module}/templates/aws-load-balancer-controller-values.yaml.tftpl", local.aws_lb_controller_config)
  ]

  depends_on = [
    module.eks,
    module.aws_load_balancer_controller_irsa
  ]
}

# External DNS
resource "helm_release" "external_dns" {
  count            = var.enable_external_dns ? 1 : 0
  name             = "external-dns"
  repository       = "https://kubernetes-sigs.github.io/external-dns/"
  chart            = "external-dns"
  version          = "1.14.3"
  namespace        = "kube-system"

  values = [
    templatefile("${path.module}/templates/external-dns-values.yaml.tftpl", local.external_dns_config)
  ]

  depends_on = [module.eks]
}

# Karpenter (optional - for advanced auto-scaling)
resource "helm_release" "karpenter" {
  count            = var.enable_karpenter ? 1 : 0
  name             = "karpenter"
  repository       = "oci://public.ecr.aws/karpenter"
  chart            = "karpenter"
  version          = "v0.33.0"
  namespace        = "karpenter"
  create_namespace = true

  values = [
    templatefile("${path.module}/templates/karpenter-values.yaml.tftpl", local.karpenter_config)
  ]

  depends_on = [module.eks]
}

# VPA (Vertical Pod Autoscaler) - optional
resource "helm_release" "vpa" {
  count            = var.enable_vpa ? 1 : 0
  name             = "vpa"
  repository       = "https://charts.fairwinds.com/stable"
  chart            = "vpa"
  version          = "3.0.0"
  namespace        = "kube-system"

  values = [
    templatefile("${path.module}/templates/vpa-values.yaml.tftpl", local.vpa_config)
  ]

  depends_on = [module.eks]
}

# Storage Classes
resource "kubernetes_storage_class" "gp3" {
  metadata {
    name = "gp3"
    annotations = {
      "storageclass.kubernetes.io/is-default-class" = "true"
    }
  }

  storage_provisioner    = "ebs.csi.aws.com"
  reclaim_policy        = "Delete"
  allow_volume_expansion = true
  volume_binding_mode   = "WaitForFirstConsumer"

  parameters = {
    type                      = "gp3"
    encrypted                 = "true"
    "csi.storage.k8s.io/fstype" = "ext4"
  }

  depends_on = [module.eks]
}

resource "kubernetes_storage_class" "gp3_retain" {
  metadata {
    name = "gp3-retain"
  }

  storage_provisioner    = "ebs.csi.aws.com"
  reclaim_policy        = "Retain"
  allow_volume_expansion = true
  volume_binding_mode   = "WaitForFirstConsumer"

  parameters = {
    type                      = "gp3"
    encrypted                 = "true"
    "csi.storage.k8s.io/fstype" = "ext4"
  }

  depends_on = [module.eks]
}