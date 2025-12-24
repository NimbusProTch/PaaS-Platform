variable "project_name" {
  description = "Project name for resource naming"
  type        = string
  default     = "infraforge"
}

variable "environment" {
  description = "Environment (dev, staging, prod)"
  type        = string
  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "Environment must be dev, staging, or prod"
  }
}

variable "tenant" {
  description = "Tenant identifier"
  type        = string
  default     = "platform"
}

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-west-1"
}

variable "owner_email" {
  description = "Owner email for tagging"
  type        = string
}

# VPC Configuration
variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "enable_nat_gateway" {
  description = "Enable NAT Gateway for private subnets"
  type        = bool
  default     = true
}

variable "single_nat_gateway" {
  description = "Use single NAT Gateway (cost optimization for non-prod)"
  type        = bool
  default     = false
}

# EKS Configuration
variable "cluster_version" {
  description = "Kubernetes version"
  type        = string
  default     = "1.30"
}

variable "node_groups" {
  description = "EKS node group configurations"
  type = map(object({
    desired_size   = number
    min_size      = number
    max_size      = number
    instance_types = list(string)
    capacity_type  = string # ON_DEMAND or SPOT
    disk_size     = number
    labels        = map(string)
    taints = list(object({
      key    = string
      value  = string
      effect = string
    }))
  }))
  default = {
    general = {
      desired_size   = 2
      min_size      = 1
      max_size      = 5
      instance_types = ["t3.large"]
      capacity_type  = "ON_DEMAND"
      disk_size     = 100
      labels = {
        workload = "general"
      }
      taints = []
    }
  }
}

# Addons
variable "enable_aws_load_balancer_controller" {
  description = "Enable AWS Load Balancer Controller"
  type        = bool
  default     = true
}

variable "enable_external_dns" {
  description = "Enable External DNS for Route53 integration"
  type        = bool
  default     = true
}

variable "enable_cluster_autoscaler" {
  description = "Enable Cluster Autoscaler"
  type        = bool
  default     = true
}

variable "enable_ebs_csi_driver" {
  description = "Enable EBS CSI Driver"
  type        = bool
  default     = true
}

# Backup
variable "enable_velero" {
  description = "Enable Velero for backup"
  type        = bool
  default     = false
}

variable "backup_bucket_name" {
  description = "S3 bucket for Velero backups"
  type        = string
  default     = ""
}

# Domain
variable "domain_name" {
  description = "Domain name for the platform"
  type        = string
  default     = ""
}

# Karpenter
variable "enable_karpenter" {
  description = "Enable Karpenter for node autoscaling"
  type        = bool
  default     = false
}

# VPA
variable "enable_vpa" {
  description = "Enable Vertical Pod Autoscaler"
  type        = bool
  default     = false
}

# GitOps and Operator Configuration

variable "github_org" {
  description = "GitHub organization name"
  type        = string
  default     = "infraforge"
}

# GitOps Configuration
variable "gitops_repo_url" {
  description = "Git repository URL for GitOps"
  type        = string
  default     = "https://github.com/infraforge/gitops-repo"
}

variable "gitops_repo_branch" {
  description = "Git repository branch for GitOps"
  type        = string
  default     = "main"
}