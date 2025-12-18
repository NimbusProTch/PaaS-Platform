#!/bin/bash

# EBS CSI Driver Fix Script
# This script fixes the EBS CSI driver issues in EKS

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== EBS CSI Driver Fix Script ===${NC}"
echo ""

# Check if AWS CLI is configured
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}Error: AWS CLI is not configured${NC}"
    exit 1
fi

# Get cluster name
CLUSTER_NAME=${1:-infraforge-dev}
REGION=${2:-eu-west-1}

echo -e "${YELLOW}Cluster: $CLUSTER_NAME${NC}"
echo -e "${YELLOW}Region: $REGION${NC}"
echo ""

# Update kubeconfig
echo -e "${GREEN}Updating kubeconfig...${NC}"
aws eks update-kubeconfig --name $CLUSTER_NAME --region $REGION

# Check current EBS CSI status
echo -e "${GREEN}Checking current EBS CSI driver status...${NC}"
kubectl get pods -n kube-system -l app=ebs-csi-controller

# Get VPC ID
VPC_ID=$(aws eks describe-cluster --name $CLUSTER_NAME --region $REGION --query 'cluster.resourcesVpcConfig.vpcId' --output text)
echo -e "${YELLOW}VPC ID: $VPC_ID${NC}"

# Check if VPC endpoints exist
echo -e "${GREEN}Checking VPC Endpoints...${NC}"

check_endpoint() {
    SERVICE=$1
    ENDPOINT=$(aws ec2 describe-vpc-endpoints \
        --filters "Name=vpc-id,Values=$VPC_ID" "Name=service-name,Values=com.amazonaws.$REGION.$SERVICE" \
        --region $REGION \
        --query 'VpcEndpoints[0].VpcEndpointId' \
        --output text)

    if [ "$ENDPOINT" != "None" ] && [ -n "$ENDPOINT" ]; then
        echo -e "${GREEN}✓ $SERVICE endpoint exists: $ENDPOINT${NC}"
        return 0
    else
        echo -e "${RED}✗ $SERVICE endpoint missing${NC}"
        return 1
    fi
}

# Check required endpoints
MISSING_ENDPOINTS=()
check_endpoint "sts" || MISSING_ENDPOINTS+=("sts")
check_endpoint "ec2" || MISSING_ENDPOINTS+=("ec2")
check_endpoint "ecr.api" || MISSING_ENDPOINTS+=("ecr.api")
check_endpoint "ecr.dkr" || MISSING_ENDPOINTS+=("ecr.dkr")
check_endpoint "s3" || MISSING_ENDPOINTS+=("s3")

# Create missing endpoints
if [ ${#MISSING_ENDPOINTS[@]} -gt 0 ]; then
    echo -e "${YELLOW}Creating missing VPC endpoints...${NC}"

    # Get subnet IDs
    SUBNET_IDS=$(aws ec2 describe-subnets \
        --filters "Name=vpc-id,Values=$VPC_ID" "Name=tag:Name,Values=*private*" \
        --region $REGION \
        --query 'Subnets[*].SubnetId' \
        --output text | tr '\t' ',')

    # Get or create security group
    SG_ID=$(aws ec2 describe-security-groups \
        --filters "Name=vpc-id,Values=$VPC_ID" "Name=group-name,Values=*endpoint*" \
        --region $REGION \
        --query 'SecurityGroups[0].GroupId' \
        --output text)

    if [ "$SG_ID" == "None" ] || [ -z "$SG_ID" ]; then
        echo -e "${YELLOW}Creating security group for VPC endpoints...${NC}"
        SG_ID=$(aws ec2 create-security-group \
            --group-name "$CLUSTER_NAME-vpc-endpoints" \
            --description "Security group for VPC endpoints" \
            --vpc-id $VPC_ID \
            --region $REGION \
            --output text)

        # Add ingress rule
        aws ec2 authorize-security-group-ingress \
            --group-id $SG_ID \
            --protocol tcp \
            --port 443 \
            --cidr 10.0.0.0/16 \
            --region $REGION
    fi

    # Create endpoints
    for ENDPOINT in "${MISSING_ENDPOINTS[@]}"; do
        echo -e "${YELLOW}Creating $ENDPOINT endpoint...${NC}"

        if [ "$ENDPOINT" == "s3" ]; then
            # S3 is a gateway endpoint
            ROUTE_TABLE_IDS=$(aws ec2 describe-route-tables \
                --filters "Name=vpc-id,Values=$VPC_ID" \
                --region $REGION \
                --query 'RouteTables[*].RouteTableId' \
                --output text | tr '\t' ',')

            aws ec2 create-vpc-endpoint \
                --vpc-id $VPC_ID \
                --service-name com.amazonaws.$REGION.s3 \
                --route-table-ids $ROUTE_TABLE_IDS \
                --region $REGION
        else
            # Interface endpoints
            aws ec2 create-vpc-endpoint \
                --vpc-id $VPC_ID \
                --service-name com.amazonaws.$REGION.$ENDPOINT \
                --vpc-endpoint-type Interface \
                --subnet-ids $SUBNET_IDS \
                --security-group-ids $SG_ID \
                --private-dns-enabled \
                --region $REGION
        fi
    done

    echo -e "${GREEN}VPC endpoints created successfully${NC}"
    echo -e "${YELLOW}Waiting 30 seconds for endpoints to be ready...${NC}"
    sleep 30
fi

# Restart CoreDNS if it's on Fargate
echo -e "${GREEN}Checking CoreDNS deployment...${NC}"
COREDNS_COMPUTE_TYPE=$(kubectl get deployment coredns -n kube-system -o jsonpath='{.spec.template.metadata.annotations.eks\.amazonaws\.com/compute-type}' 2>/dev/null || echo "")

if [ "$COREDNS_COMPUTE_TYPE" == "fargate" ]; then
    echo -e "${YELLOW}CoreDNS is running on Fargate, patching to run on EC2...${NC}"

    kubectl patch deployment coredns -n kube-system --type='json' -p='[
        {
            "op": "remove",
            "path": "/spec/template/metadata/annotations/eks.amazonaws.com~1compute-type"
        }
    ]' || true

    kubectl patch deployment coredns -n kube-system --patch '{
        "spec": {
            "template": {
                "spec": {
                    "affinity": {
                        "nodeAffinity": {
                            "requiredDuringSchedulingIgnoredDuringExecution": {
                                "nodeSelectorTerms": [{
                                    "matchExpressions": [{
                                        "key": "eks.amazonaws.com/compute-type",
                                        "operator": "NotIn",
                                        "values": ["fargate"]
                                    }]
                                }]
                            }
                        }
                    }
                }
            }
        }
    }'

    echo -e "${GREEN}CoreDNS patched to run on EC2 nodes${NC}"
fi

# Restart CoreDNS
echo -e "${GREEN}Restarting CoreDNS...${NC}"
kubectl rollout restart deployment coredns -n kube-system
kubectl rollout status deployment coredns -n kube-system --timeout=300s

# Clear CoreDNS cache
echo -e "${GREEN}Clearing CoreDNS cache...${NC}"
kubectl get pods -n kube-system -l k8s-app=kube-dns -o name | xargs -I {} kubectl delete {} -n kube-system
sleep 10

# Check EBS CSI driver addon
echo -e "${GREEN}Checking EBS CSI driver addon...${NC}"
ADDON_STATUS=$(aws eks describe-addon --cluster-name $CLUSTER_NAME --addon-name aws-ebs-csi-driver --region $REGION --query 'addon.status' --output text 2>/dev/null || echo "NOT_FOUND")

if [ "$ADDON_STATUS" == "NOT_FOUND" ]; then
    echo -e "${YELLOW}EBS CSI driver addon not found. Please install it via Terraform.${NC}"
else
    echo -e "${GREEN}EBS CSI driver addon status: $ADDON_STATUS${NC}"

    if [ "$ADDON_STATUS" != "ACTIVE" ]; then
        echo -e "${YELLOW}Restarting EBS CSI driver pods...${NC}"
        kubectl rollout restart deployment ebs-csi-controller -n kube-system
        kubectl rollout restart daemonset ebs-csi-node -n kube-system

        echo -e "${YELLOW}Waiting for EBS CSI driver to be ready...${NC}"
        kubectl wait --for=condition=Ready pods -l app=ebs-csi-controller -n kube-system --timeout=300s
    fi
fi

# Create default storage class
echo -e "${GREEN}Creating default storage class...${NC}"
cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Delete
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3-retain
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Retain
EOF

# Test EBS CSI driver
echo -e "${GREEN}Testing EBS CSI driver with a test PVC...${NC}"
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ebs-test-pvc
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: gp3
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Pod
metadata:
  name: ebs-test-pod
  namespace: default
spec:
  containers:
  - name: test
    image: busybox
    command: ["sleep", "3600"]
    volumeMounts:
    - name: test-volume
      mountPath: /data
  volumes:
  - name: test-volume
    persistentVolumeClaim:
      claimName: ebs-test-pvc
EOF

echo -e "${YELLOW}Waiting for test pod to be ready...${NC}"
sleep 10
kubectl wait --for=condition=Ready pod/ebs-test-pod -n default --timeout=120s || true

# Check test results
PVC_STATUS=$(kubectl get pvc ebs-test-pvc -n default -o jsonpath='{.status.phase}' 2>/dev/null || echo "Failed")
POD_STATUS=$(kubectl get pod ebs-test-pod -n default -o jsonpath='{.status.phase}' 2>/dev/null || echo "Failed")

echo ""
echo -e "${GREEN}=== Test Results ===${NC}"
echo -e "PVC Status: $PVC_STATUS"
echo -e "Pod Status: $POD_STATUS"

if [ "$PVC_STATUS" == "Bound" ] && [ "$POD_STATUS" == "Running" ]; then
    echo -e "${GREEN}✓ EBS CSI driver is working correctly!${NC}"

    # Cleanup test resources
    echo -e "${YELLOW}Cleaning up test resources...${NC}"
    kubectl delete pod ebs-test-pod -n default --force --grace-period=0 2>/dev/null || true
    kubectl delete pvc ebs-test-pvc -n default 2>/dev/null || true
else
    echo -e "${RED}✗ EBS CSI driver test failed${NC}"
    echo -e "${YELLOW}Checking logs for troubleshooting...${NC}"
    kubectl logs -n kube-system -l app=ebs-csi-controller --tail=50
fi

echo ""
echo -e "${GREEN}=== Fix Script Completed ===${NC}"
echo ""
echo -e "${YELLOW}Summary:${NC}"
echo "1. VPC endpoints checked and created if missing"
echo "2. CoreDNS moved from Fargate to EC2 nodes"
echo "3. CoreDNS cache cleared"
echo "4. EBS CSI driver restarted"
echo "5. Default storage classes created"
echo "6. EBS CSI driver tested"

echo ""
echo -e "${YELLOW}If issues persist, check:${NC}"
echo "- kubectl logs -n kube-system -l app=ebs-csi-controller"
echo "- kubectl describe pod -n kube-system -l app=ebs-csi-controller"
echo "- aws eks describe-addon --cluster-name $CLUSTER_NAME --addon-name aws-ebs-csi-driver --region $REGION"