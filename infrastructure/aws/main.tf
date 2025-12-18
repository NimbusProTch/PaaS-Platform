# Provider configurations
provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Environment = var.environment
      ManagedBy   = "OpenTofu"
      Platform    = "InfraForge"
      Owner       = var.owner_email
    }
  }
}

# Data sources
data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_caller_identity" "current" {}

# Locals
locals {
  cluster_name = "${var.project_name}-${var.environment}"

  common_tags = {
    Cluster     = local.cluster_name
    Environment = var.environment
    Tenant      = var.tenant
  }

  # AZ selection for multi-AZ deployment
  azs = slice(data.aws_availability_zones.available.names, 0, 3)
}