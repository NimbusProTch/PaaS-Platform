# GitOps Configuration Repository

This repository contains all application and infrastructure configurations managed by ArgoCD.

## Structure

```
.
├── applications/          # Business application Helm charts
│   ├── backend-service/
│   ├── frontend/
│   └── ...
├── components/           # Infrastructure components
│   ├── postgresql/
│   ├── redis/
│   ├── mongodb/
│   ├── elasticsearch/
│   ├── kafka/
│   └── rabbitmq/
├── environments/         # Environment-specific overrides
│   ├── development/
│   ├── staging/
│   └── production/
└── teams/               # Team-specific configurations
    ├── e-commerce/
    ├── analytics/
    ├── mobile/
    └── ml/
```

## How It Works

1. **Platform Operator** creates ArgoCD Applications
2. **ArgoCD** watches this repository
3. **ArgoCD** deploys all resources to Kubernetes
4. **Auto-sync** ensures cluster state matches Git

## ArgoCD Integration

All deployments are managed through ArgoCD with:
- Automated sync
- Self-healing
- Prune resources
- Revision history

## Component Management

Infrastructure components use Bitnami Helm charts:
- PostgreSQL
- Redis
- MongoDB
- Elasticsearch
- Kafka
- RabbitMQ