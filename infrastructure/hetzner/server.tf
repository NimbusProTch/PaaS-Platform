# server.tf - Hetzner Cloud Server resource

resource "hcloud_ssh_key" "default" {
  name       = "${var.server_name}-key"
  public_key = file(pathexpand(var.ssh_public_key_path))
}

resource "hcloud_server" "dev_server" {
  name        = var.server_name
  server_type = var.server_type
  location    = var.location
  image       = var.image

  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.dev_server.id]

  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    enable_kind       = var.enable_kind
    enable_vagrant    = var.enable_vagrant
    enable_rke        = var.enable_rke
    enable_go         = var.enable_go
    enable_nodejs     = var.enable_nodejs
    enable_ansible    = var.enable_ansible
    enable_claude     = var.enable_claude
    anthropic_api_key = var.anthropic_api_key
    git_repo_url      = var.git_repo_url
  })

  labels = {
    environment = "dev"
    purpose     = "development-workstation"
    project     = "infraforge"
    managed_by  = "opentofu"
  }

  lifecycle {
    ignore_changes = [
      user_data, # Don't recreate on cloud-init changes
    ]
  }
}
