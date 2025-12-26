# variables.tf - Input variables

# ==============================================================================
# REQUIRED VARIABLES
# ==============================================================================

variable "hcloud_token" {
  description = "Hetzner Cloud API Token (export HCLOUD_TOKEN=xxx)"
  type        = string
  sensitive   = true
}

# ==============================================================================
# SERVER CONFIGURATION
# ==============================================================================

variable "server_name" {
  description = "Server name"
  type        = string
  default     = "infraforge-dev"
}

variable "server_type" {
  description = "Server type: cx22(4GB), cx32(8GB), cx42(16GB), cx52(32GB), ccx13(8GB), ccx23(16GB), ccx33(32GB), ccx43(64GB)"
  type        = string
  default     = "ccx33"
}

variable "location" {
  description = "Datacenter location: fsn1 (Falkenstein), nbg1 (Nuremberg), hel1 (Helsinki)"
  type        = string
  default     = "fsn1"
}

variable "image" {
  description = "OS image"
  type        = string
  default     = "ubuntu-24.04"
}

# ==============================================================================
# SSH CONFIGURATION
# ==============================================================================

variable "ssh_public_key_path" {
  description = "Path to SSH public key"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "ssh_private_key_path" {
  description = "Path to SSH private key (for provisioners)"
  type        = string
  default     = "~/.ssh/id_rsa"
}

# ==============================================================================
# PROJECT CONFIGURATION
# ==============================================================================

variable "git_repo_url" {
  description = "Git repository to clone"
  type        = string
  default     = "https://github.com/NimbusProTch/PaaS-Platform.git"
}

variable "workspace_dir" {
  description = "Workspace directory on server"
  type        = string
  default     = "/root/workspace"
}

# ==============================================================================
# FEATURE TOGGLES
# ==============================================================================

variable "enable_kind" {
  description = "Install Kind and create Kubernetes cluster (includes kubectl, helm, k9s)"
  type        = bool
  default     = false
}

variable "enable_vagrant" {
  description = "Install Vagrant and Libvirt for nested VMs (for RKE clusters)"
  type        = bool
  default     = false
}

variable "enable_rke" {
  description = "Install RKE2 CLI (requires enable_vagrant=true for VM-based clusters)"
  type        = bool
  default     = false
}

variable "enable_go" {
  description = "Install Go compiler (for operator development)"
  type        = bool
  default     = true
}

variable "enable_nodejs" {
  description = "Install Node.js, npm, yarn, pnpm"
  type        = bool
  default     = true
}

variable "enable_ansible" {
  description = "Install Ansible and ansible-lint"
  type        = bool
  default     = true
}

variable "enable_claude" {
  description = "Install Claude CLI (requires ANTHROPIC_API_KEY)"
  type        = bool
  default     = true
}

variable "anthropic_api_key" {
  description = "Anthropic API key for Claude CLI (optional, can be set later)"
  type        = string
  default     = ""
  sensitive   = true
}
