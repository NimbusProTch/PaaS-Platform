# Microservice CI/CD Setup Guide

This guide explains how to set up CI/CD pipelines for microservices using the template workflow.

## Overview

The microservice CI/CD pipeline provides:
- Automated testing (unit tests, linting, security scanning)
- Docker image building for amd64 architecture
- Push to Amazon ECR
- Release package creation
- Notifications

## Prerequisites

### 1. AWS IAM Role Setup

Create an IAM role for GitHub Actions with the following permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecr:GetAuthorizationToken",
        "ecr:BatchCheckLayerAvailability",
        "ecr:GetDownloadUrlForLayer",
        "ecr:BatchGetImage",
        "ecr:PutImage",
        "ecr:InitiateLayerUpload",
        "ecr:UploadLayerPart",
        "ecr:CompleteLayerUpload"
      ],
      "Resource": "*"
    }
  ]
}
```

Trust policy for GitHub Actions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::715841344657:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:infraforge/*:*"
        }
      }
    }
  ]
}
```

### 2. ECR Repositories

Create ECR repositories for each microservice:

```bash
# Create repositories
aws ecr create-repository --repository-name infraforge-dev/product-service --region eu-west-1
aws ecr create-repository --repository-name infraforge-dev/user-service --region eu-west-1
aws ecr create-repository --repository-name infraforge-dev/order-service --region eu-west-1
aws ecr create-repository --repository-name infraforge-dev/payment-service --region eu-west-1
aws ecr create-repository --repository-name infraforge-dev/notification-service --region eu-west-1
```

## Setup Instructions

### For Each Microservice Repository:

#### 1. Copy the Workflow Template

Copy the `microservice-template.yaml` file to each microservice repository:

```bash
# In the microservice repository
mkdir -p .github/workflows
cp /path/to/platform-operator/.github/workflows/microservice-template.yaml .github/workflows/ci-cd.yaml
```

#### 2. Configure GitHub Secrets

Add the following secrets to the repository (Settings → Secrets and variables → Actions):

Required secrets:
- `AWS_ROLE_ARN`: ARN of the IAM role created above (e.g., `arn:aws:iam::715841344657:role/github-actions-role`)

Optional secrets:
- `SLACK_WEBHOOK`: Slack webhook URL for notifications

#### 3. Create Dockerfile

Each microservice needs a Dockerfile. Here's a template for Go services:

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary for linux/amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/service ./cmd/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/service .

# Expose port (adjust as needed)
EXPOSE 8080

CMD ["./service"]
```

#### 4. Test the Workflow

Push a commit to trigger the workflow:

```bash
git add .github/workflows/ci-cd.yaml Dockerfile
git commit -m "Add CI/CD pipeline"
git push origin main
```

#### 5. Create a Release

To create a release package:

```bash
# Tag the commit
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

This will:
1. Build and push the image with tag `v1.0.0`
2. Create a GitHub release
3. Generate a release manifest

## Workflow Stages

### 1. Test
- Runs unit tests
- Generates coverage reports
- Uploads to Codecov

### 2. Lint
- Runs golangci-lint
- Checks code quality

### 3. Security Scan
- Scans code for vulnerabilities using Trivy
- Uploads results to GitHub Security tab

### 4. Build
- Builds Docker image for linux/amd64
- Pushes to ECR with multiple tags:
  - Branch name (e.g., `main`, `develop`)
  - Commit SHA
  - Version tag (if tagged)
  - `latest` (for main branch)
- Scans the built image for vulnerabilities

### 5. Create Release (on tags only)
- Generates changelog
- Creates release manifest with image reference
- Creates GitHub release

### 6. Notify
- Sends Slack notifications (if configured)
- Reports build status

## Release Package Format

The release manifest created for each version:

```yaml
apiVersion: platform.infraforge.io/v1
kind: ReleasePackage
metadata:
  name: product-service
  version: v1.0.0
spec:
  image: 715841344657.dkr.ecr.eu-west-1.amazonaws.com/infraforge-dev/product-service:v1.0.0
  commit: abc123def456
  buildDate: 2025-12-20T21:00:00Z
  tested: true
  approved: false
```

## Using Release Packages with the Platform

To deploy a release package, update your ApplicationClaim:

```yaml
apiVersion: platform.infraforge.io/v1
kind: ApplicationClaim
metadata:
  name: ecommerce-platform
spec:
  applications:
    - name: product-service
      repository: https://github.com/infraforge/product-service
      version: v1.0.0  # Use the release version
      image: 715841344657.dkr.ecr.eu-west-1.amazonaws.com/infraforge-dev/product-service
      # ... other config
```

## Troubleshooting

### ECR Authentication Fails

Check that:
1. AWS IAM role ARN is correct in secrets
2. OIDC provider is configured in AWS
3. Trust policy allows your repository

### Image Build Fails

Check that:
1. Dockerfile exists in repository root
2. Go version matches (1.21)
3. Build architecture is set to amd64

### Tests Fail

Ensure:
1. All test dependencies are in go.mod
2. Tests pass locally
3. Test data/fixtures are committed

## Best Practices

1. **Semantic Versioning**: Use semantic versioning for tags (v1.0.0, v1.0.1, etc.)
2. **Branch Protection**: Protect main/develop branches, require PR reviews
3. **Test Coverage**: Aim for >80% test coverage
4. **Security Scanning**: Fix HIGH/CRITICAL vulnerabilities before merging
5. **Release Notes**: Write meaningful git commit messages for changelog generation

## Next Steps

After setting up CI/CD:
1. Configure branch protection rules
2. Set up code owners (CODEOWNERS file)
3. Configure required status checks
4. Enable Dependabot for dependency updates
5. Set up automated deployments to staging/prod
