environment = "dev"
aws_region = "eu-west-1"
owner_email = "platform@infraforge.io"
tenant = "platform"

# VPC Configuration
vpc_cidr = "10.0.0.0/16"
enable_nat_gateway = true
single_nat_gateway = true # Cost optimization for dev

# EKS Configuration
cluster_version = "1.28"

node_groups = {
  general = {
    desired_size   = 2
    min_size      = 1
    max_size      = 3
    instance_types = ["t3.medium"]
    capacity_type  = "SPOT" # Use SPOT instances for dev
    disk_size     = 50
    labels = {
      workload = "general"
    }
    taints = []
  }
}

# No dedicated database nodes in dev
enable_database_nodes = false

# Addons
enable_aws_load_balancer_controller = true
enable_external_dns = false # Not needed in dev
enable_cert_manager = false # Not needed in dev
enable_metrics_server = true
enable_cluster_autoscaler = true
enable_ebs_csi_driver = true

# Storage
storage_class_parameters = {
  type      = "gp3"
  iops      = 3000
  encrypted = "true"
}

# Monitoring (basic in dev)
enable_prometheus = true
enable_grafana = false

# Backup disabled in dev
enable_velero = false

# Platform Operator
enable_argocd = true