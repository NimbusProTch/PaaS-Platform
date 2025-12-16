# AWS EKS Deployment Guide

## Prerequisites

### 1. Install Required Tools

```bash
# AWS CLI
brew install awscli  # macOS
# or
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"

# OpenTofu (recommended) or Terraform
brew install opentofu  # macOS
# or
brew install terraform

# kubectl
brew install kubectl

# Helm
brew install helm
```

### 2. Configure AWS Credentials

```bash
aws configure
# Enter your:
# - AWS Access Key ID
# - AWS Secret Access Key
# - Default region (eu-west-1)
# - Default output format (json)
```

Or use environment variables:
```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_DEFAULT_REGION="eu-west-1"
```

### 3. Check AWS Permissions

Your AWS user/role needs these permissions:
- EKS full access
- VPC full access
- IAM create roles
- EC2 full access
- S3 (for state storage)
- KMS (for encryption)

## 🚀 Quick Start

### One-Command Deployment

```bash
chmod +x deploy-aws.sh
./deploy-aws.sh dev  # For development environment
# or
./deploy-aws.sh prod # For production environment
```

This script will:
1. Create VPC with public/private subnets
2. Create EKS cluster
3. Install all operators
4. Deploy ArgoCD
5. Configure platform services
6. Set up auto-sync

## 📝 Manual Deployment Steps

### Step 1: Initialize Terraform/OpenTofu

```bash
cd infrastructure/aws
tofu init  # or terraform init
```

### Step 2: Review Configuration

Edit `environments/dev.tfvars` or `environments/prod.tfvars`:

```hcl
environment = "dev"
aws_region = "eu-west-1"
owner_email = "your-email@example.com"

# Adjust node configuration
node_groups = {
  general = {
    desired_size   = 2
    min_size      = 1
    max_size      = 3
    instance_types = ["t3.medium"]  # Change instance type
    capacity_type  = "SPOT"          # Use ON_DEMAND for production
    disk_size     = 50
  }
}
```

### Step 3: Plan Infrastructure

```bash
tofu plan -var-file=environments/dev.tfvars
```

Review the resources that will be created:
- VPC with CIDR 10.0.0.0/16
- 3 public subnets
- 3 private subnets
- Internet Gateway
- NAT Gateway(s)
- EKS cluster
- Managed node group(s)
- IAM roles and policies

### Step 4: Create Infrastructure

```bash
tofu apply -var-file=environments/dev.tfvars -auto-approve
```

⏱️ This takes about 10-15 minutes

### Step 5: Update kubeconfig

```bash
aws eks update-kubeconfig --name infraforge-dev --region eu-west-1
```

### Step 6: Verify Connection

```bash
kubectl get nodes
```

### Step 7: Install Platform

```bash
cd ../..  # Back to root directory

# Install operators
kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.23/releases/cnpg-1.23.0.yaml
kubectl apply -f https://github.com/rabbitmq/cluster-operator/releases/latest/download/cluster-operator.yml
helm install redis-operator ot-helm/redis-operator --create-namespace --namespace redis-operator-system
kubectl apply -k 'https://github.com/minio/operator/resources?ref=v6.0.0'

# Install ArgoCD
kubectl create namespace infraforge-argocd
kubectl apply -n infraforge-argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Create namespaces
kubectl create namespace demo-dev

# Deploy ApplicationSets
kubectl apply -f manifests/platform-cluster/appsets/dev/operator.yaml
```

## 💰 Cost Optimization

### Development Environment
- **Estimated Cost**: ~$150-200/month
- Uses SPOT instances (70% savings)
- Single NAT gateway
- Minimal replicas
- Auto-scaling down to 1 node

### Production Environment
- **Estimated Cost**: ~$800-1200/month
- ON_DEMAND instances for stability
- Multi-AZ NAT gateways
- HA configurations
- Auto-scaling 3-10 nodes

### Cost Saving Tips

1. **Use SPOT instances for dev/test**
```hcl
capacity_type = "SPOT"
```

2. **Schedule cluster scaling**
```bash
# Scale down at night
kubectl scale deployment --all --replicas=0 -n demo-dev
```

3. **Use single NAT gateway for dev**
```hcl
single_nat_gateway = true
```

4. **Enable cluster autoscaler**
```hcl
enable_cluster_autoscaler = true
```

## 🔍 Monitoring and Access

### Access ArgoCD

```bash
# Port forward
kubectl port-forward svc/argocd-server -n infraforge-argocd 8080:443

# Get admin password
kubectl -n infraforge-argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Open browser
open https://localhost:8080
```

### Check Service Status

```bash
# All pods
kubectl get pods -A

# Platform services
kubectl get pods -n demo-dev

# ArgoCD applications
kubectl get applications -n infraforge-argocd

# Operators
kubectl get pods -A | grep operator
```

### View Logs

```bash
# ArgoCD logs
kubectl logs -n infraforge-argocd deployment/argocd-applicationset-controller -f

# Service logs
kubectl logs -n demo-dev <pod-name> -f
```

## 🛠️ Troubleshooting

### Issue: Pods Pending

```bash
# Check node capacity
kubectl describe nodes

# Check events
kubectl get events -n demo-dev --sort-by='.lastTimestamp'
```

### Issue: ArgoCD Not Syncing

```bash
# Force refresh
kubectl -n infraforge-argocd patch application <app-name> \
  --type merge -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}'
```

### Issue: Insufficient Resources

```bash
# Scale up nodes
cd infrastructure/aws
tofu apply -var-file=environments/dev.tfvars \
  -var='node_groups={"general":{"desired_size":3,"min_size":2,"max_size":5}}'
```

## 🧹 Clean Up

### Delete Services Only

```bash
kubectl delete namespace demo-dev
kubectl delete applications -n infraforge-argocd --all
```

### Delete Everything (Including EKS)

```bash
cd infrastructure/aws
tofu destroy -var-file=environments/dev.tfvars -auto-approve
```

⚠️ **Warning**: This deletes ALL resources including:
- EKS cluster
- VPC and networking
- All data in the cluster

## 📊 Resource Usage

### Development Defaults

| Component | Instances | CPU | Memory | Storage |
|-----------|-----------|-----|--------|---------|
| EKS Nodes | 2 x t3.medium | 2 vCPU | 4 GiB | 50 GiB |
| PostgreSQL | 1 | 500m | 1Gi | 10Gi |
| RabbitMQ | 1 | 500m | 1Gi | 10Gi |
| Redis | 1 | 100m | 256Mi | - |
| MinIO | 1 | 500m | 1Gi | 10Gi |
| Vault | 1 | 100m | 256Mi | 5Gi |

### Production Defaults

| Component | Instances | CPU | Memory | Storage |
|-----------|-----------|-----|--------|---------|
| EKS Nodes | 3 x t3.xlarge | 4 vCPU | 16 GiB | 100 GiB |
| DB Nodes | 2 x r6i.xlarge | 4 vCPU | 32 GiB | 200 GiB |
| PostgreSQL | 2-3 | 500m-2000m | 1-4Gi | 20Gi |
| RabbitMQ | 2-3 | 250m-500m | 512Mi-1Gi | 5Gi |
| Redis | 1 | 100m-500m | 256Mi-512Mi | 5Gi |
| MinIO | 2 | 250m-500m | 512Mi-1Gi | 10Gi |
| Vault | 1 | 100m-500m | 256Mi-512Mi | 5Gi |

## 🚦 Next Steps

After deployment:

1. **Set up monitoring**
   ```bash
   make install-prometheus
   make install-grafana
   ```

2. **Configure DNS**
   - Point your domain to the Load Balancer
   - Enable External DNS

3. **Set up backups**
   ```bash
   make install-velero
   ```

4. **Install Backstage** (after platform is stable)
   ```bash
   make install-backstage
   ```

## 📞 Support

- GitHub Issues: https://github.com/NimbusProTch/PaaS-Platform/issues
- Documentation: See PLATFORM_ARCHITECTURE.md
- AWS Support: Check your support plan

## ✅ Checklist

Before going to production:

- [ ] Configure S3 backend for Terraform state
- [ ] Set up proper AWS IAM roles (not root)
- [ ] Configure cluster autoscaling
- [ ] Enable monitoring and alerts
- [ ] Set up backup strategy
- [ ] Configure network policies
- [ ] Enable audit logging
- [ ] Set up cost alerts
- [ ] Document runbooks
- [ ] Test disaster recovery