environment = "prod"
aws_region = "eu-west-1"
owner_email = "platform@infraforge.io"
tenant = "platform"

# VPC Configuration
vpc_cidr = "10.0.0.0/16"
enable_nat_gateway = true
single_nat_gateway = false # Multi-AZ NAT for HA

# EKS Configuration
cluster_version = "1.28"

node_groups = {
  general = {
    desired_size   = 3
    min_size      = 3
    max_size      = 10
    instance_types = ["t3.xlarge", "t3a.xlarge"]
    capacity_type  = "ON_DEMAND"
    disk_size     = 100
    labels = {
      workload = "general"
    }
    taints = []
  }
}

# Enable dedicated database nodes in prod
enable_database_nodes = true

# Addons
enable_aws_load_balancer_controller = true
enable_external_dns = true
enable_cert_manager = true
enable_metrics_server = true
enable_cluster_autoscaler = true
enable_ebs_csi_driver = true

# Storage
storage_class_parameters = {
  type      = "gp3"
  iops      = 10000
  encrypted = "true"
}

# Monitoring (full stack in prod)
enable_prometheus = true
enable_grafana = true

# Backup enabled in prod
enable_velero = true
backup_bucket_name = "infraforge-prod-backups"

# Domain configuration
domain_name = "infraforge.io"
create_route53_zone = true

# Backstage
enable_backstage = true
backstage_github_org = "NimbusProTch"