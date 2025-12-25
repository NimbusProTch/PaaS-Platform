# infrastructure/hetzner/main.tf
#
# InfraForge - Hetzner Cloud Development Server
# 
# Tam donanımlı remote development ortamı:
# ✅ Docker & Docker Compose
# ✅ Kind (Kubernetes in Docker)
# ✅ Kubectl, Helm, K9s
# ✅ Vagrant + Libvirt (nested VMs)
# ✅ Ansible
# ✅ OpenTofu
# ✅ Git + Repo auto-clone
# ✅ VS Code Server ready
# ✅ Lens bağlantısı için kubeconfig
# ✅ Tüm portlar açık (dev ortamı)
#
# Kullanım:
#   export HCLOUD_TOKEN="your-token"
#   tofu init
#   tofu apply      # ~3 dakika
#   tofu destroy    # Para durur!
#

terraform {
  required_version = ">= 1.6.0"
  
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}

# ============================================================================
# VARIABLES
# ============================================================================

variable "hcloud_token" {
  description = "Hetzner Cloud API Token (export HCLOUD_TOKEN=xxx)"
  type        = string
  sensitive   = true
}

variable "server_name" {
  description = "Server name"
  type        = string
  default     = "infraforge-dev"
}

variable "server_type" {
  description = "Server type: ccx13(8GB), ccx23(16GB), ccx33(32GB), ccx43(64GB)"
  type        = string
  default     = "ccx33"  # 32GB RAM, 8 vCPU - €0.0672/saat
}

variable "location" {
  description = "Datacenter: fsn1, nbg1, hel1"
  type        = string
  default     = "fsn1"
}

variable "ssh_public_key_path" {
  description = "Path to SSH public key"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "ssh_private_key_path" {
  description = "Path to SSH private key"
  type        = string
  default     = "~/.ssh/id_rsa"
}

variable "git_repo_url" {
  description = "Git repository to clone"
  type        = string
  default     = "https://github.com/NimbusProTch/PaaS-Platform.git"
}

variable "workspace_dir" {
  description = "Workspace directory"
  type        = string
  default     = "/root/workspace"
}

# ============================================================================
# FEATURE TOGGLES
# ============================================================================

variable "enable_kind" {
  description = "Install Kind and create Kubernetes cluster"
  type        = bool
  default     = false
}

variable "enable_vagrant" {
  description = "Install Vagrant and Libvirt for nested VMs"
  type        = bool
  default     = false
}

variable "enable_go" {
  description = "Install Go compiler (for operator development)"
  type        = bool
  default     = true
}

variable "enable_nodejs" {
  description = "Install Node.js and npm"
  type        = bool
  default     = true
}

variable "enable_ansible" {
  description = "Install Ansible"
  type        = bool
  default     = true
}

# ============================================================================
# PROVIDER
# ============================================================================

provider "hcloud" {
  token = var.hcloud_token
}

# ============================================================================
# SSH KEY
# ============================================================================

resource "hcloud_ssh_key" "default" {
  name       = "${var.server_name}-key"
  public_key = file(pathexpand(var.ssh_public_key_path))
}

# ============================================================================
# FIREWALL - TÜM PORTLAR AÇIK (Dev ortamı)
# ============================================================================

resource "hcloud_firewall" "dev_server" {
  name = "${var.server_name}-fw"

  # SSH
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # HTTP
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "80"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # HTTPS
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # Kubernetes API
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "6443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # Gitea
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "3000"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # ArgoCD
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "8080"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # Kubernetes Dashboard
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "8443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # NodePort range
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "30000-32767"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # Any high port (for testing)
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "8000-9999"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # ICMP (ping)
  rule {
    direction  = "in"
    protocol   = "icmp"
    source_ips = ["0.0.0.0/0", "::/0"]
  }
}

# ============================================================================
# SERVER
# ============================================================================

resource "hcloud_server" "dev_server" {
  name        = var.server_name
  server_type = var.server_type
  location    = var.location
  image       = "ubuntu-24.04"
  
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.dev_server.id]

  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    enable_kind    = var.enable_kind
    enable_vagrant = var.enable_vagrant
    enable_go      = var.enable_go
    enable_nodejs  = var.enable_nodejs
    enable_ansible = var.enable_ansible
    git_repo_url   = var.git_repo_url
  })

  labels = {
    environment = "dev"
    purpose     = "development-workstation"
    project     = "infraforge"
  }
}

# ============================================================================
# OUTPUTS
# ============================================================================

output "server_ip" {
  description = "Server public IP"
  value       = hcloud_server.dev_server.ipv4_address
}

output "ssh_command" {
  description = "SSH command"
  value       = "ssh root@${hcloud_server.dev_server.ipv4_address}"
}

output "setup_instructions" {
  description = "Setup instructions"
  value       = <<-EOF

  InfraForge Dev Server Ready!
  ============================

  SSH: ssh root@${hcloud_server.dev_server.ipv4_address}

  VS Code Remote SSH (~/.ssh/config):
    Host infraforge-dev
      HostName ${hcloud_server.dev_server.ipv4_address}
      User root
      ForwardAgent yes

  Enabled features:
    - Kind (K8s): ${var.enable_kind}
    - Vagrant:    ${var.enable_vagrant}
    - Go:         ${var.enable_go}
    - Node.js:    ${var.enable_nodejs}
    - Ansible:    ${var.enable_ansible}
${var.enable_kind ? "\n  Lens kubeconfig:\n    scp root@${hcloud_server.dev_server.ipv4_address}:/root/.kube/config-external ~/.kube/infraforge-dev\n" : ""}
  Cost: ~0.067/hour (${var.server_type})
  Don't forget: tofu destroy

  EOF
}

output "vscode_ssh_config" {
  description = "Add this to ~/.ssh/config"
  value       = <<-EOF

Host infraforge-dev
  HostName ${hcloud_server.dev_server.ipv4_address}
  User root
  ForwardAgent yes
  StrictHostKeyChecking no

  EOF
}

output "lens_kubeconfig_command" {
  description = "Command to get kubeconfig for Lens (only when Kind enabled)"
  value       = var.enable_kind ? "scp root@${hcloud_server.dev_server.ipv4_address}:/root/.kube/config-external ~/.kube/infraforge-dev" : "Kind not enabled - no kubeconfig available"
}

output "hourly_cost" {
  description = "Hourly cost reminder"
  value       = "💰 ~€0.067/hour (${var.server_type}). Don't forget: tofu destroy"
}

output "workspace_path" {
  description = "Workspace path on server"
  value       = "/root/workspace/PaaS-Platform"
}
