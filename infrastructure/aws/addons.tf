# EKS Add-ons and Essential Components

# Metrics Server
resource "helm_release" "metrics_server" {
  name             = "metrics-server"
  repository       = "https://kubernetes-sigs.github.io/metrics-server/"
  chart            = "metrics-server"
  version          = "3.12.0"
  namespace        = "kube-system"

  values = [
    <<-EOT
    args:
      - --kubelet-insecure-tls
      - --kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname

    resources:
      requests:
        cpu: 100m
        memory: 200Mi
      limits:
        cpu: 200m
        memory: 400Mi
    EOT
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
    <<-EOT
    autoDiscovery:
      clusterName: ${local.cluster_name}
      enabled: true

    awsRegion: ${var.aws_region}

    rbac:
      serviceAccount:
        annotations:
          eks.amazonaws.com/role-arn: ${module.cluster_autoscaler_irsa[0].iam_role_arn}
        name: cluster-autoscaler
        create: true

    extraArgs:
      skip-nodes-with-local-storage: false
      skip-nodes-with-system-pods: false
      balance-similar-node-groups: true
      expander: least-waste

    resources:
      requests:
        cpu: 100m
        memory: 300Mi
      limits:
        cpu: 200m
        memory: 500Mi
    EOT
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
    <<-EOT
    clusterName: ${local.cluster_name}
    region: ${var.aws_region}
    vpcId: ${module.vpc.vpc_id}

    serviceAccount:
      create: true
      name: aws-load-balancer-controller
      annotations:
        eks.amazonaws.com/role-arn: ${module.aws_load_balancer_controller_irsa[0].iam_role_arn}

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

# External DNS
resource "helm_release" "external_dns" {
  count            = var.enable_external_dns ? 1 : 0
  name             = "external-dns"
  repository       = "https://kubernetes-sigs.github.io/external-dns/"
  chart            = "external-dns"
  version          = "1.14.3"
  namespace        = "kube-system"

  values = [
    <<-EOT
    provider: aws

    aws:
      region: ${var.aws_region}
      zoneType: public

    domainFilters:
      - ${var.domain_name}

    policy: sync

    serviceAccount:
      create: true
      name: external-dns
      annotations:
        eks.amazonaws.com/role-arn: ${module.external_dns_irsa[0].iam_role_arn}

    resources:
      requests:
        cpu: 50m
        memory: 100Mi
      limits:
        cpu: 100m
        memory: 200Mi
    EOT
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
    <<-EOT
    settings:
      clusterName: ${local.cluster_name}
      clusterEndpoint: ${module.eks.cluster_endpoint}
      interruptionQueueName: ${local.cluster_name}
      aws:
        # defaultInstanceProfile: ${var.karpenter_instance_profile_name}  # TODO: Add Karpenter IRSA module
        vmMemoryOverheadPercent: 0.075

    serviceAccount:
      annotations:
        # eks.amazonaws.com/role-arn: ${var.karpenter_irsa_role_arn}  # TODO: Add Karpenter IRSA module

    replicas: 2

    resources:
      requests:
        cpu: 1
        memory: 1Gi
      limits:
        cpu: 2
        memory: 2Gi
    EOT
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
    <<-EOT
    recommender:
      resources:
        requests:
          cpu: 50m
          memory: 100Mi
        limits:
          cpu: 100m
          memory: 200Mi

    updater:
      enabled: false  # We'll only use recommendations, not auto-updates

    admissionController:
      resources:
        requests:
          cpu: 50m
          memory: 100Mi
        limits:
          cpu: 100m
          memory: 200Mi
    EOT
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