# Backend Service

A simple Go-based backend service for testing the platform operator.

## Features

- Health check endpoint at `/health`
- Info endpoint at `/info` showing environment variables
- Root endpoint at `/` showing service status

## Build

```bash
# Build Docker image
docker build -t backend-service:v1.0.0 .

# Run locally
docker run -p 8080:8080 backend-service:v1.0.0
```

## Environment Variables

- `PORT`: Service port (default: 8080)
- `APP_VERSION`: Application version (default: v1.0.0)
- `DATABASE_URL`: PostgreSQL connection string
- `REDIS_URL`: Redis connection string
- `ENVIRONMENT`: Deployment environment (development/staging/production)

## Kubernetes Deployment

The `k8s/` directory contains Kubernetes manifests for deployment:
- `deployment.yaml`: Deployment configuration
- `service.yaml`: Service configuration

## API Endpoints

- `GET /`: Service status
- `GET /health`: Health check
- `GET /info`: Service information and environment