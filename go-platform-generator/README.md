# Go Platform Generator for Kratix

This is a Go-based generator for creating Kratix promises and pipelines for platform applications.

## Architecture

```
Infrastructure (Terragrunt)
    ↓
Root App (ArgoCD)
    ↓
ApplicationSet
    ↓
Kratix Claims → Platform Apps
```

## Structure

- **Promise**: Defines the API for platform stacks
- **Pipeline**: Processes claims and generates ArgoCD applications
- **Components**: Platform applications (Redis, Nginx, PostgreSQL, Keycloak)

## How it works

1. User creates a PlatformStack claim specifying which components to enable
2. Kratix pipeline processes the claim using this generator
3. Generator creates ArgoCD Application manifests for each enabled component
4. ArgoCD deploys the applications with proper namespacing and configuration

## Example Claim

```yaml
apiVersion: platform.example.com/v1
kind: PlatformStack
metadata:
  name: demo-team-dev
spec:
  tenant: demo-team
  environment: dev
  components:
    redis:
      enabled: true
    nginx:
      enabled: true
```

This will create:
- Namespace: `redis-demo-team-dev` with Redis deployment
- Namespace: `nginx-demo-team-dev` with Nginx deployment
- ArgoCD Applications managing these deployments

## Building

```bash
go build -o generator cmd/generator/main.go
```

## Docker Image

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o generator cmd/generator/main.go

FROM alpine:3.18
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/generator /app/generator
ENTRYPOINT ["/app/generator"]
```