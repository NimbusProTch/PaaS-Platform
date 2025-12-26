# InfraForge Dev Server - Hetzner Cloud

Remote development server for InfraForge platform development.

## Quick Start

```bash
# 1. Get Hetzner API Token
#    https://console.hetzner.cloud/ -> Security -> API Tokens -> Generate

# 2. Deploy
cd infrastructure/hetzner
export HCLOUD_TOKEN="your-token"
tofu init
tofu apply

# 3. Connect via VS Code
#    Add SSH config from output, then Remote-SSH: Connect to Host

# 4. Destroy when done (stops billing!)
tofu destroy
```

## File Structure

```
infrastructure/hetzner/
├── versions.tf           # Terraform/OpenTofu version constraints
├── variables.tf          # Input variables & feature toggles
├── provider.tf           # Hetzner Cloud provider
├── firewall.tf           # Firewall rules
├── server.tf             # Server resource
├── outputs.tf            # Output values
├── cloud-init.yaml.tftpl # Cloud-init template
├── terraform.tfvars      # Your configuration
└── README.md             # This file
```

## Feature Toggles

Edit `terraform.tfvars` to enable/disable features:

| Feature | Default | Description |
|---------|---------|-------------|
| `enable_kind` | false | Kind + Kubectl + Helm + K9s |
| `enable_vagrant` | false | Vagrant + Libvirt (for RKE VMs) |
| `enable_rke` | false | RKE2 CLI |
| `enable_go` | true | Go 1.23 compiler |
| `enable_nodejs` | true | Node.js 22 + npm + yarn + pnpm |
| `enable_ansible` | true | Ansible + ansible-lint |
| `enable_claude` | true | Claude CLI |

## Server Types

| Type | RAM | CPU | Cost/hour |
|------|-----|-----|-----------|
| cx22 | 4GB | 2 | ~0.01 |
| cx32 | 8GB | 4 | ~0.02 |
| ccx13 | 8GB | 2 | ~0.03 |
| ccx23 | 16GB | 4 | ~0.05 |
| ccx33 | 32GB | 8 | ~0.07 |
| ccx43 | 64GB | 16 | ~0.13 |

## Daily Workflow

```
Morning:  tofu apply
Work:     VS Code Remote SSH
Evening:  tofu destroy  (billing stops!)
```

## Estimated Cost

- 8 hours/day: ~0.54/day
- 20 days/month: ~10-15/month
- Weekends off: 0
