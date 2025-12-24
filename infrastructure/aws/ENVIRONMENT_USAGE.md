# Environment-Specific Usage

## Dev Environment
```bash
tofu plan -var-file=environments/dev.tfvars
tofu apply -var-file=environments/dev.tfvars
```

## Prod Environment
```bash
tofu plan -var-file=environments/prod.tfvars
tofu apply -var-file=environments/prod.tfvars
```
