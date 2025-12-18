# AWS Infrastructure Configuration

# Basic Configuration
project_name = "infraforge"
environment  = "dev"
tenant       = "platform"
owner_email  = "gokhan@infraforge.io"

# AWS Configuration
aws_region = "eu-west-1"

# EKS Configuration
cluster_version = "1.29"

# VPC Configuration
vpc_cidr           = "10.0.0.0/16"
enable_nat_gateway = true
single_nat_gateway = true # Set to false for production

# Node Groups Configuration
node_groups = {
  general = {
    instance_types = ["t3.medium"]
    capacity_type  = "SPOT"
    min_size       = 2
    max_size       = 5
    desired_size   = 2
    disk_size      = 50
    labels = {
      workload = "general"
    }
    taints = []
  }
}

# Database Node Group (for production)
# node_groups = {
#   general = {
#     instance_types = ["t3.xlarge"]
#     capacity_type  = "ON_DEMAND"
#     min_size       = 3
#     max_size       = 10
#     desired_size   = 3
#     disk_size      = 100
#     labels = {
#       workload = "general"
#     }
#     taints = []
#   }
#   database = {
#     instance_types = ["r6i.xlarge"]
#     capacity_type  = "ON_DEMAND"
#     min_size       = 2
#     max_size       = 4
#     desired_size   = 2
#     disk_size      = 200
#     labels = {
#       workload = "database"
#     }
#     taints = [
#       {
#         key    = "workload"
#         value  = "database"
#         effect = "NoSchedule"
#       }
#     ]
#   }
# }

# Feature Flags
enable_cluster_autoscaler           = true
enable_aws_load_balancer_controller = true
enable_external_dns                 = false
enable_cert_manager                 = true
enable_ebs_csi_driver              = true
enable_velero                      = false
enable_karpenter                   = false
enable_vpa                         = false

# Monitoring
enable_monitoring = true
monitoring_config = {
  prometheus_retention_days = 30
  prometheus_storage_size   = "50Gi"
  grafana_admin_password   = "" # Will be auto-generated if empty
}

# DNS Configuration (for external-dns)
domain_name = "infraforge.io"

# Backup Configuration (for Velero)
backup_bucket_name = "infraforge-dev-backups"