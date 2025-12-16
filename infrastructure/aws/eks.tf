# EKS Cluster
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

  # Addons
  cluster_addons = {
    coredns = {
      most_recent = true
      configuration_values = jsonencode({
        computeType = "Fargate"
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

  # Fargate profiles for system workloads
  fargate_profiles = {
    system = {
      selectors = [
        {
          namespace = "kube-system"
          labels = {
            k8s-app = "kube-dns"
          }
        },
        {
          namespace = "cert-manager"
        },
        {
          namespace = "external-dns"
        }
      ]
    }
  }

  # Node groups
  eks_managed_node_groups = {
    # General purpose nodes
    general = {
      name = "${local.cluster_name}-general"

      instance_types = var.node_groups.general.instance_types
      capacity_type  = var.node_groups.general.capacity_type

      min_size     = var.node_groups.general.min_size
      max_size     = var.node_groups.general.max_size
      desired_size = var.node_groups.general.desired_size

      disk_size = var.node_groups.general.disk_size

      labels = var.node_groups.general.labels
      taints = var.node_groups.general.taints

      update_config = {
        max_unavailable_percentage = 33
      }

      tags = local.common_tags
    }
  }

  # Database nodes (if enabled for production)
  dynamic "eks_managed_node_groups" {
    for_each = var.enable_database_nodes ? { database = true } : {}

    content {
      database = {
        name = "${local.cluster_name}-database"

        instance_types = ["r6i.xlarge", "r6i.2xlarge"]
        capacity_type  = "ON_DEMAND"

        min_size     = 1
        max_size     = 3
        desired_size = 2

        disk_size = 200

        labels = {
          workload = "database"
          tier     = "data"
        }

        taints = [
          {
            key    = "workload"
            value  = "database"
            effect = "NO_SCHEDULE"
          }
        ]

        tags = merge(
          local.common_tags,
          {
            Type = "database"
          }
        )
      }
    }
  }

  # Encryption
  cluster_encryption_config = {
    provider_key_arn = aws_kms_key.eks.arn
    resources        = ["secrets"]
  }

  tags = local.common_tags
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