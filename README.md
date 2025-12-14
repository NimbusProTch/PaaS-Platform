# InfraForge - Enterprise Platform as a Service

**Production-ready, enterprise-grade Platform-as-a-Service (PaaS)** for on-premise data centers that enables self-service deployment of applications, databases, and middleware with GitOps principles.

## 🚀 Features

- **Self-Service Platform**: Teams can deploy services with simple YAML declarations
- **GitOps Workflow**: All changes tracked in Git with automatic deployment
- **Multi-Tenancy**: Namespace isolation with resource quotas
- **Environment Management**: Separate dev/staging/prod configurations
- **Service Profiles**: Pre-configured profiles for different use cases
- **Automated Operations**: Backup, monitoring, and security configured automatically

## 📋 Prerequisites

- Docker
- Kind (Kubernetes in Docker)
- kubectl
- Helm
- envsubst (usually comes with gettext package)

## 🛠️ Quick Start

### 1. Set GitHub Credentials

```bash
export GITHUB_USERNAME=your-github-username
export GITHUB_TOKEN=your-personal-access-token
```

### 2. Deploy Platform

```bash
make all
```

This single command will:
- Create a Kind cluster
- Install cert-manager and Kratix
- Deploy ArgoCD with proper RBAC
- Configure GitHub integration
- Create ArgoCD projects (dev/staging/prod)
- Deploy the InfraForge Promise
- Set up the bootstrap application

### 3. Deploy a Service

```bash
# Deploy nginx service
kubectl apply -f claims/test-nginx-v2.yaml
```

### 4. Check Status

```bash
make status
```

### 5. Access ArgoCD UI

```bash
make port-forward-argocd
# Open http://localhost:8080
# Username: admin
# Password: (shown in terminal)
```

## 📦 Supported Services

### Business Applications
- **nginx**: Web server and reverse proxy
- **backoffice**: Business application template
- **frontend**: Frontend application template
- **webapp**: Web application template

### Platform Services
- **vault**: HashiCorp Vault for secrets management
- **istio**: Service mesh
- **keycloak**: Identity and access management
- **grafana**: Observability platform

### Database Operators
- **postgresql**: Relational database (CloudNativePG operator)
- **redis**: In-memory data store (Redis operator)
- **mongodb**: NoSQL database
- **kafka**: Event streaming platform
- **rabbitmq**: Message broker

## 🏗️ Architecture

```
Developer → InfraForge CR → Kratix Pipeline → Git → ArgoCD ApplicationSets → Kubernetes
```

**How it works:**
1. Developer creates an InfraForge Custom Resource
2. Kratix pipeline (Go generator) processes the request
3. Generates Helm charts, ApplicationSets, and operator CRs
4. Kratix syncs manifests to Git repository
5. ArgoCD ApplicationSets discover and deploy resources
6. Services are deployed to Kubernetes with proper isolation

## 📁 Repository Structure

```
manifests/voltron/
├── <tenant>-<env>/
│   ├── argocd/<tenant>/<env>/
│   │   └── services-appset.yaml      # ApplicationSets for services
│   └── apps/<tenant>/<env>/
│       └── <service>/
│           └── <service>-application.yaml  # ArgoCD Application
```

## 🎯 Example: Deploy MongoDB

```yaml
apiVersion: platform.infraforge.io/v1
kind: InfraForge
metadata:
  name: my-database
  namespace: default
spec:
  tenant: myteam
  environment: dev
  services:
  - name: userdb
    type: mongodb
    profile: dev
```

## 🔧 Makefile Targets

```bash
make help              # Show all available commands
make all               # Full platform setup
make clean             # Delete cluster and clean resources
make status            # Show platform status
make test              # Deploy test application
make logs              # Show pipeline logs
make port-forward-argocd  # Access ArgoCD UI
```

## 🐛 Troubleshooting

### ArgoCD Sync Issues

If ArgoCD shows "Unknown" sync status:
1. Check if GitHub credentials are correct
2. Manually refresh the application in ArgoCD UI
3. Check ArgoCD logs: `kubectl logs -n infraforge-argocd deployment/argocd-repo-server`

### Pipeline Not Running

Check Kratix logs:
```bash
kubectl logs -n kratix-platform-system -l platform.kratix.io/pipeline-name --tail=100
```

### Work Status

Check work status:
```bash
kubectl get works -A
kubectl describe work <work-name> -n <namespace>
```

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- [Kratix](https://kratix.io/) - Platform orchestration
- [ArgoCD](https://argoproj.github.io/cd/) - GitOps continuous delivery
- [Kind](https://kind.sigs.k8s.io/) - Local Kubernetes clusters
EOF < /dev/null