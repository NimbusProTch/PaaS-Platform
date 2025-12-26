# outputs.tf - Output values

output "server_ip" {
  description = "Server public IP address"
  value       = hcloud_server.dev_server.ipv4_address
}

output "ssh_command" {
  description = "SSH command to connect"
  value       = "ssh root@${hcloud_server.dev_server.ipv4_address}"
}

output "vscode_ssh_config" {
  description = "Add this to ~/.ssh/config for VS Code Remote SSH"
  value       = <<-EOF

Host ${var.server_name}
  HostName ${hcloud_server.dev_server.ipv4_address}
  User root
  ForwardAgent yes
  StrictHostKeyChecking no

  EOF
}

output "enabled_features" {
  description = "Enabled feature toggles"
  value = {
    kind    = var.enable_kind
    vagrant = var.enable_vagrant
    rke     = var.enable_rke
    go      = var.enable_go
    nodejs  = var.enable_nodejs
    ansible = var.enable_ansible
    claude  = var.enable_claude
  }
}

output "lens_kubeconfig_command" {
  description = "Command to get kubeconfig for Lens (only when Kind enabled)"
  value       = var.enable_kind ? "scp root@${hcloud_server.dev_server.ipv4_address}:/root/.kube/config-external ~/.kube/${var.server_name}" : null
}

output "hourly_cost" {
  description = "Estimated hourly cost"
  value       = "~0.067/hour (${var.server_type})"
}

output "workspace_path" {
  description = "Workspace path on server"
  value       = "/root/workspace/PaaS-Platform"
}

output "next_steps" {
  description = "Next steps after server is ready"
  value       = <<-EOF

  1. Wait ~3-5 minutes for cloud-init to complete
  2. SSH: ssh root@${hcloud_server.dev_server.ipv4_address}
  3. Check status: cat /root/setup-status.txt
  4. VS Code: Add SSH config above, then Cmd+Shift+P -> Remote-SSH: Connect

  Don't forget: tofu destroy (when done)

  EOF
}
