# EBS CSI Controller Troubleshooting Guide

## Issue Description
EBS CSI Controller pods were experiencing `CrashLoopBackOff` with the following error:
```
dial tcp: lookup sts.eu-west-1.amazonaws.com: i/o timeout
```

## Root Cause Analysis

### 1. Missing VPC Endpoints
**Problem**: The VPC configuration was missing critical endpoints for AWS services that the EBS CSI driver needs to communicate with.

**Services Required**:
- **STS (Security Token Service)**: Required for IRSA authentication
- **EC2**: Required for EBS volume operations

**Solution**: Add VPC endpoints for STS and EC2 services in `vpc.tf`:
```hcl
# VPC Endpoint for STS (required for EBS CSI Driver IRSA)
resource "aws_vpc_endpoint" "sts" {
  vpc_id              = module.vpc.vpc_id
  service_name        = "com.amazonaws.${var.aws_region}.sts"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = module.vpc.private_subnets
  security_group_ids  = [aws_security_group.vpc_endpoints.id]
  private_dns_enabled = true
}

# VPC Endpoint for EC2 (required for EBS CSI Driver)
resource "aws_vpc_endpoint" "ec2" {
  vpc_id              = module.vpc.vpc_id
  service_name        = "com.amazonaws.${var.aws_region}.ec2"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = module.vpc.private_subnets
  security_group_ids  = [aws_security_group.vpc_endpoints.id]
  private_dns_enabled = true
}
```

### 2. CoreDNS on Fargate
**Problem**: CoreDNS is configured to run on Fargate, which can cause DNS resolution delays and issues.

**Current Configuration** in `eks.tf`:
```hcl
cluster_addons = {
  coredns = {
    most_recent = true
    configuration_values = jsonencode({
      computeType = "Fargate"
    })
  }
}
```

**Recommended Solution**: Move CoreDNS to managed node groups for better reliability:
```hcl
cluster_addons = {
  coredns = {
    most_recent = true
    # Remove Fargate configuration to run on EC2 nodes
  }
}
```

### 3. Deployment Order Dependencies
**Problem**: No explicit dependency between VPC endpoints and EBS CSI driver addon deployment.

**Solution**: Add explicit dependencies in the EKS module or use a separate resource for EBS CSI deployment:
```hcl
resource "aws_eks_addon" "ebs_csi" {
  cluster_name = module.eks.cluster_name
  addon_name   = "aws-ebs-csi-driver"

  depends_on = [
    aws_vpc_endpoint.sts,
    aws_vpc_endpoint.ec2,
    module.ebs_csi_driver_irsa
  ]
}
```

## Prevention Strategies

### 1. Complete VPC Endpoint Configuration
Always include all necessary VPC endpoints for EKS addons:
- S3 (Gateway endpoint) ✅
- ECR API ✅
- ECR DKR ✅
- STS ✅ (Added during fix)
- EC2 ✅ (Added during fix)
- ELB (for AWS Load Balancer Controller)
- AutoScaling (for Cluster Autoscaler)

### 2. CoreDNS Best Practices
- Run CoreDNS on EC2 nodes instead of Fargate for production workloads
- Configure proper resource limits and replicas
- Use node selectors to ensure CoreDNS runs on stable nodes

### 3. Testing Strategy
Before deploying to production:
1. Deploy infrastructure
2. Verify all VPC endpoints are created: `aws ec2 describe-vpc-endpoints`
3. Check CoreDNS is running: `kubectl get pods -n kube-system -l k8s-app=kube-dns`
4. Test DNS resolution from within cluster: `kubectl run test --image=busybox --rm -it -- nslookup sts.eu-west-1.amazonaws.com`
5. Verify EBS CSI controller health: `kubectl get pods -n kube-system -l app=ebs-csi-controller`

### 4. Monitoring and Alerts
Set up monitoring for:
- EBS CSI controller pod restarts
- DNS resolution latency
- VPC endpoint health
- IRSA authentication failures

## Quick Diagnostics Commands

```bash
# Check EBS CSI Controller status
kubectl get pods -n kube-system -l app=ebs-csi-controller

# View EBS CSI Controller logs
kubectl logs -n kube-system -l app=ebs-csi-controller

# Test DNS resolution from within cluster
kubectl run dns-test --image=busybox --rm -it -- nslookup sts.eu-west-1.amazonaws.com

# Verify VPC endpoints
aws ec2 describe-vpc-endpoints --filters "Name=vpc-id,Values=<VPC_ID>"

# Check IRSA configuration
kubectl describe sa ebs-csi-controller-sa -n kube-system
```

## Emergency Recovery Steps

If EBS CSI controller fails:
1. Check VPC endpoints are created and healthy
2. Restart CoreDNS pods: `kubectl rollout restart deployment coredns -n kube-system`
3. Clear DNS cache if using NodeLocal DNSCache
4. Restart EBS CSI controller: `kubectl rollout restart deployment ebs-csi-controller -n kube-system`
5. If issues persist, recreate the addon:
   ```bash
   aws eks delete-addon --cluster-name <cluster> --addon-name aws-ebs-csi-driver
   # Wait for deletion
   aws eks create-addon --cluster-name <cluster> --addon-name aws-ebs-csi-driver --service-account-role-arn <IRSA_ARN>
   ```