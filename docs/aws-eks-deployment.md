# AWS EKS Production Deployment Guide

## Prerequisites

Ensure you have the following tools installed:
- AWS CLI (configured with credentials)
- OpenTofu/Terraform (>= 1.5.0)
- kubectl (>= 1.28)
- Helm (>= 3.12.0)

## 1. Infrastructure Deployment

### Configure Variables

Create a `terraform.tfvars` file in the `infrastructure/aws` directory:

```hcl
project_name = "infraforge"
environment  = "dev"      # or "staging", "prod"
tenant       = "platform"
aws_region   = "eu-west-1"
owner_email  = "your-email@example.com"

# VPC Configuration
vpc_cidr           = "10.0.0.0/16"
enable_nat_gateway = true
single_nat_gateway = true  # Use false for prod (multi-AZ)

# EKS Configuration
cluster_version = "1.28"

# Node Groups
node_groups = {
  general = {
    desired_size   = 3
    min_size      = 2
    max_size      = 10
    instance_types = ["t3.large"]
    capacity_type  = "ON_DEMAND"  # or "SPOT" for cost savings
    disk_size     = 100
    labels = {
      workload = "general"
    }
    taints = []
  }
}

# Enable Production Components
enable_aws_load_balancer_controller = true
enable_external_dns                 = true
enable_cert_manager                 = true
enable_metrics_server               = true
enable_cluster_autoscaler          = true
enable_ebs_csi_driver              = true
enable_kong                        = true
enable_prometheus                  = true
enable_grafana                     = true

# Optional Components
enable_loki   = false  # Enable for log aggregation
enable_tempo  = false  # Enable for distributed tracing
enable_velero = false  # Enable for backup

# Domain Configuration (optional)
domain_name         = "infraforge.io"
create_route53_zone = false  # Set to true if you want to create a new zone
```

### Deploy Infrastructure

```bash
cd infrastructure/aws

# Initialize Terraform
tofu init

# Review the plan
tofu plan

# Apply the infrastructure
tofu apply

# Save the kubeconfig
aws eks update-kubeconfig --region eu-west-1 --name infraforge-dev
```

## 2. Component Installation

After the EKS cluster is created, the Terraform configuration will automatically install:

### Core Components
- **Kong API Gateway**: Ingress controller with Gateway API support
- **AWS Load Balancer Controller**: For ALB/NLB provisioning
- **EBS CSI Driver**: For persistent volumes with gp3 storage
- **Metrics Server**: For HPA and kubectl top
- **Cluster Autoscaler**: For automatic node scaling

### Optional Components (based on variables)
- **External DNS**: Automatic Route53 record management
- **Cert Manager**: Automatic TLS certificate management
- **Prometheus & Grafana**: Monitoring and observability
- **Loki**: Log aggregation
- **Tempo**: Distributed tracing

## 3. Manual Installation (Alternative)

If you prefer to install components manually or selectively:

```bash
# Set environment variables
export CLUSTER_NAME=infraforge-dev
export AWS_REGION=eu-west-1
export ENVIRONMENT=dev

# Run the installation script
./scripts/install-eks-components.sh

# For production with monitoring
INSTALL_MONITORING=true ./scripts/install-eks-components.sh

# With External DNS and Cert Manager
ENABLE_EXTERNAL_DNS=true ENABLE_CERT_MANAGER=true ./scripts/install-eks-components.sh
```

## 4. Verify Installation

### Check Core Components

```bash
# Check nodes
kubectl get nodes

# Check metrics
kubectl top nodes
kubectl top pods -A

# Check storage classes
kubectl get storageclass

# Check Kong Gateway
kubectl get gateway -A
kubectl get gatewayclass
kubectl get svc -n kong kong-proxy

# Check Load Balancer Controller
kubectl get deployment -n kube-system aws-load-balancer-controller

# Check Cluster Autoscaler
kubectl get deployment -n kube-system cluster-autoscaler
```

### Access Kong Gateway

```bash
# Get the Load Balancer URL
kubectl get svc -n kong kong-proxy -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
```

### Access Grafana (if enabled)

```bash
# Get Grafana URL
kubectl get svc -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'

# Get admin password
kubectl get secret -n monitoring kube-prometheus-stack-grafana -o jsonpath="{.data.admin-password}" | base64 -d
```

## 5. Deploy Applications

### Using Gateway API

Create an HTTPRoute for your application:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app-route
  namespace: default
spec:
  parentRefs:
  - name: platform-gateway
    namespace: default
  hostnames:
  - my-app.infraforge.io
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: my-app-service
      port: 80
```

### Using Kong Plugins

Apply rate limiting to a route:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app-route
  namespace: default
  annotations:
    konghq.com/plugins: rate-limiting
spec:
  # ... route configuration
```

## 6. Deploy Platform Operators

Deploy the operators and applications:

```bash
# Deploy operators using ArgoCD ApplicationSet
kubectl apply -f manifests/platform-cluster/operators/applicationset.yaml

# Deploy applications
kubectl apply -f manifests/platform-cluster/apps/
```

## 7. Monitoring & Observability

### Prometheus Queries

Access Prometheus UI or use Grafana to query metrics:

```promql
# Node CPU usage
100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)

# Pod memory usage
sum by (pod) (container_memory_working_set_bytes)

# Kong request rate
sum(rate(kong_http_requests_total[5m]))
```

### Pre-configured Dashboards

The following dashboards are automatically imported:
- Kubernetes Cluster Overview (7249)
- Kubernetes Pods (6417)
- Node Exporter (11074)
- Kong Dashboard (7424)
- Kong Gateway API (13115)

## 8. Cost Optimization

### Use Spot Instances

Update node groups to use SPOT capacity:

```hcl
node_groups = {
  spot = {
    capacity_type = "SPOT"
    instance_types = ["t3.large", "t3a.large", "t2.large"]
    # ... other configuration
  }
}
```

### Enable Karpenter (Advanced)

For better autoscaling and cost optimization:

```hcl
enable_karpenter = true
```

### Use Single NAT Gateway (Non-Prod)

```hcl
single_nat_gateway = true  # Saves ~$45/month per additional NAT
```

## 9. Security Best Practices

### Network Policies

Apply network policies to restrict pod communication:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
  namespace: default
spec:
  podSelector: {}
  policyTypes:
  - Ingress
```

### RBAC

Create service accounts with minimal permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: app-sa
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: app-role
  namespace: default
rules:
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: app-rolebinding
  namespace: default
subjects:
- kind: ServiceAccount
  name: app-sa
roleRef:
  kind: Role
  name: app-role
  apiGroup: rbac.authorization.k8s.io
```

## 10. Troubleshooting

### Common Issues

**Pods Pending**: Check node resources
```bash
kubectl describe pod <pod-name>
kubectl top nodes
```

**LoadBalancer Pending**: Check AWS Load Balancer Controller
```bash
kubectl logs -n kube-system deployment/aws-load-balancer-controller
```

**Storage Issues**: Check EBS CSI Driver
```bash
kubectl get csidriver
kubectl get pvc -A
```

**Gateway Not Working**: Check Kong logs
```bash
kubectl logs -n kong deployment/kong-gateway
```

### Clean Up

To destroy all resources:

```bash
cd infrastructure/aws
tofu destroy
```

## Next Steps

1. **Configure DNS**: Point your domain to the Kong Load Balancer
2. **Enable TLS**: Use cert-manager with Let's Encrypt
3. **Deploy Applications**: Use ArgoCD ApplicationSets
4. **Setup Monitoring Alerts**: Configure Prometheus alerting rules
5. **Implement Backup**: Enable Velero for disaster recovery
6. **Add Backstage**: Deploy developer portal (coming next)

## Support

For issues or questions:
- Check logs: `kubectl logs -n <namespace> <pod>`
- Describe resources: `kubectl describe <resource> <name>`
- Check events: `kubectl get events -A --sort-by='.lastTimestamp'`