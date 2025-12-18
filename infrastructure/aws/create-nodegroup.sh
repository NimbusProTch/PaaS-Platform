#!/bin/bash

# Create EKS Node Group
CLUSTER_NAME="infraforge-dev"
REGION="eu-west-1"
NODEGROUP_NAME="general-nodes"

# Get subnet IDs
SUBNET_IDS=$(aws ec2 describe-subnets \
  --filters "Name=vpc-id,Values=$(aws eks describe-cluster --name $CLUSTER_NAME --region $REGION --query 'cluster.resourcesVpcConfig.vpcId' --output text)" \
  --filters "Name=tag:Name,Values=*private*" \
  --region $REGION \
  --query 'Subnets[*].SubnetId' \
  --output text | tr '\t' ',')

echo "Creating node group for cluster: $CLUSTER_NAME"
echo "Using subnets: $SUBNET_IDS"

# Create node group
aws eks create-nodegroup \
  --cluster-name $CLUSTER_NAME \
  --nodegroup-name $NODEGROUP_NAME \
  --subnets $(echo $SUBNET_IDS | tr ',' ' ') \
  --instance-types t3.medium \
  --ami-type AL2_x86_64 \
  --node-role arn:aws:iam::715841344657:role/infraforge-dev-general-eks-node-group-20251217170752624000000007 \
  --scaling-config minSize=2,maxSize=5,desiredSize=2 \
  --disk-size 50 \
  --region $REGION

echo "Node group creation initiated. Check status with:"
echo "aws eks describe-nodegroup --cluster-name $CLUSTER_NAME --nodegroup-name $NODEGROUP_NAME --region $REGION"