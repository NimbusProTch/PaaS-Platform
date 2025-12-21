# EKS Cluster - FIXED Configuration
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.21"

  cluster_name    = local.cluster_name
  cluster_version = var.cluster_version

  # Network config
  vpc_id                   = module.vpc.vpc_id
  subnet_ids               = module.vpc.private_subnets
  control_plane_subnet_ids = module.vpc.private_subnets

  # API endpoint access
  cluster_endpoint_private_access = true
  cluster_endpoint_public_access  = true
  cluster_endpoint_public_access_cidrs = ["0.0.0.0/0"] # Restrict in production

  # Addons - CoreDNS'i EC2 node'larda çalıştır
  cluster_addons = {
    coredns = {
      most_recent = true
      # Fargate'den kaldırıyoruz, EC2 node'larda çalışacak
      configuration_values = jsonencode({
        replicaCount = 2
        resources = {
          limits = {
            cpu    = "100m"
            memory = "128Mi"
          }
          requests = {
            cpu    = "100m"
            memory = "70Mi"
          }
        }
        affinity = {
          nodeAffinity = {
            requiredDuringSchedulingIgnoredDuringExecution = {
              nodeSelectorTerms = [{
                matchExpressions = [{
                  key      = "eks.amazonaws.com/compute-type"
                  operator = "NotIn"
                  values   = ["fargate"]
                }]
              }]
            }
          }
        }
      })
    }
    kube-proxy = {
      most_recent = true
    }
    vpc-cni = {
      most_recent = true
      configuration_values = jsonencode({
        env = {
          ENABLE_PREFIX_DELEGATION = "true"
        }
      })
    }
    aws-ebs-csi-driver = {
      most_recent = var.enable_ebs_csi_driver
      service_account_role_arn = var.enable_ebs_csi_driver ? module.ebs_csi_driver_irsa[0].iam_role_arn : null
    }
  }

  # OIDC Provider
  enable_irsa = true

  # Cluster access
  manage_aws_auth_configmap = true
  aws_auth_roles = [
    {
      rolearn  = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/Admin"
      username = "admin"
      groups   = ["system:masters"]
    }
  ]

  # Fargate profiles - CoreDNS'i kaldırıyoruz
  fargate_profiles = {
    system = {
      selectors = [
        {
          namespace = "cert-manager"
        },
        {
          namespace = "external-dns"
        },
        {
          namespace = "velero"
        }
      ]
    }
  }

  # Node groups
  eks_managed_node_groups = {
    # General purpose nodes - EBS CSI driver için güncellendi
    general = {
      name = "${local.cluster_name}-general"

      instance_types = var.node_groups.general.instance_types
      capacity_type  = var.node_groups.general.capacity_type

      min_size     = var.node_groups.general.min_size
      max_size     = var.node_groups.general.max_size
      desired_size = var.node_groups.general.desired_size

      disk_size = var.node_groups.general.disk_size

      labels = merge(
        var.node_groups.general.labels,
        {
          "node-type" = "general"
        }
      )

      taints = var.node_groups.general.taints

      # User data for EBS CSI driver optimization
      pre_bootstrap_user_data = <<-EOT
        #!/bin/bash
        # Optimize EBS performance
        echo "vm.dirty_ratio = 5" >> /etc/sysctl.conf
        echo "vm.dirty_background_ratio = 1" >> /etc/sysctl.conf
        sysctl -p
      EOT

      update_config = {
        max_unavailable_percentage = 33
      }

      tags = local.common_tags
    }

    # System nodes for critical components
    system = {
      name = "${local.cluster_name}-system"

      instance_types = ["t3.medium"]
      capacity_type  = "ON_DEMAND"

      min_size     = 2
      max_size     = 3
      desired_size = 2

      disk_size = 50

      labels = {
        "node-type" = "system"
        "workload"  = "core-services"
      }

      taints = []

      tags = local.common_tags
    }
  }

  # Encryption
  cluster_encryption_config = {
    provider_key_arn = aws_kms_key.eks.arn
    resources        = ["secrets"]
  }

  tags = local.common_tags

  # Ensure VPC endpoints are created before EKS
  depends_on = [
    aws_vpc_endpoint.sts,
    aws_vpc_endpoint.ec2,
    aws_vpc_endpoint.ecr_api,
    aws_vpc_endpoint.ecr_dkr
  ]
}

# KMS key for EKS cluster encryption
resource "aws_kms_key" "eks" {
  description             = "KMS key for ${local.cluster_name} EKS cluster encryption"
  deletion_window_in_days = 10
  enable_key_rotation     = true

  tags = merge(
    local.common_tags,
    {
      Name = "${local.cluster_name}-eks-key"
    }
  )
}

resource "aws_kms_alias" "eks" {
  name          = "alias/${local.cluster_name}-eks"
  target_key_id = aws_kms_key.eks.key_id
}

# IRSA for cluster autoscaler
module "cluster_autoscaler_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.30"

  count = var.enable_cluster_autoscaler ? 1 : 0

  role_name = "${local.cluster_name}-cluster-autoscaler"

  attach_cluster_autoscaler_policy = true
  cluster_autoscaler_cluster_names = [module.eks.cluster_name]

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["kube-system:cluster-autoscaler"]
    }
  }

  tags = local.common_tags
}

# IRSA for AWS Load Balancer Controller
module "aws_load_balancer_controller_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.30"

  count = var.enable_aws_load_balancer_controller ? 1 : 0

  role_name = "${local.cluster_name}-aws-load-balancer-controller"

  attach_load_balancer_controller_policy = true

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["kube-system:aws-load-balancer-controller"]
    }
  }

  tags = local.common_tags
}

# IRSA for External DNS
module "external_dns_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.30"

  count = var.enable_external_dns ? 1 : 0

  role_name = "${local.cluster_name}-external-dns"

  attach_external_dns_policy = true

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["external-dns:external-dns"]
    }
  }

  tags = local.common_tags
}

# IRSA for EBS CSI Driver - IMPROVED
module "ebs_csi_driver_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.30"

  count = var.enable_ebs_csi_driver ? 1 : 0

  role_name = "${local.cluster_name}-ebs-csi-driver"

  attach_ebs_csi_policy = true

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["kube-system:ebs-csi-controller-sa"]
    }
  }

  tags = local.common_tags
}

# IRSA for Velero backup
module "velero_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.30"

  count = var.enable_velero ? 1 : 0

  role_name = "${local.cluster_name}-velero"

  attach_velero_policy = true
  velero_s3_bucket_arns = [
    "arn:aws:s3:::${var.backup_bucket_name}",
    "arn:aws:s3:::${var.backup_bucket_name}/*"
  ]

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["velero:velero"]
    }
  }

  tags = local.common_tags
}

# Post-deployment configuration for EBS CSI
resource "null_resource" "configure_ebs_csi" {
  count = var.enable_ebs_csi_driver ? 1 : 0

  provisioner "local-exec" {
    command = <<-EOT
      aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.aws_region}

      # Wait for EBS CSI driver to be ready
      kubectl wait --for=condition=Ready pods -l app=ebs-csi-controller -n kube-system --timeout=300s || true

      # Create default storage class if it doesn't exist
      kubectl apply -f - <<EOF
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
      EOF
    EOT
  }

  depends_on = [module.eks]
}