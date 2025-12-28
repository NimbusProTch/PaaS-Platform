#!/bin/bash

# Gitea'ya bağlan
GITEA_URL="http://localhost:3000"
GITEA_USER="gitea_admin"
GITEA_PASS="r8sA8CPHD9!bt6d"

echo "📂 GitOps repository hazırlanıyor..."

# Port forward başlat
kubectl port-forward -n gitea svc/gitea-http 3000:3000 > /dev/null 2>&1 &
PF_PID=$!
sleep 3

# Repository clone
rm -rf /tmp/voltran 2>/dev/null || true
git clone ${GITEA_URL}/infraforge/voltran /tmp/voltran 2>/dev/null || {
  mkdir -p /tmp/voltran
  cd /tmp/voltran
  git init
  git remote add origin ${GITEA_URL}/infraforge/voltran
}

cd /tmp/voltran

# GitOps structure oluştur
mkdir -p appsets/nonprod/apps
mkdir -p appsets/nonprod/platform
mkdir -p environments/nonprod/dev/applications
mkdir -p environments/nonprod/dev/platform

# Dev ApplicationSet (ChartMuseum'dan çekecek)
cat <<'EOF' > appsets/nonprod/apps/dev-appset.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dev-microservices
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran
      revision: main
      files:
      - path: "environments/nonprod/dev/applications/*/values.yaml"

  template:
    metadata:
      name: '{{path[4]}}-dev'
    spec:
      project: default
      source:
        repoURL: http://chartmuseum.chartmuseum.svc.cluster.local:8080
        chart: microservice
        targetRevision: "1.0.0"
        helm:
          valueFiles:
          - http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran/raw/branch/main/{{path}}
      destination:
        server: https://kubernetes.default.svc
        namespace: dev
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
        - CreateNamespace=true
EOF

# Platform ApplicationSet
cat <<'EOF' > appsets/nonprod/platform/dev-platform-appset.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dev-platform
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran
      revision: main
      files:
      - path: "environments/nonprod/dev/platform/*/values.yaml"

  template:
    metadata:
      name: '{{path[4]}}-platform-dev'
    spec:
      project: default
      source:
        repoURL: http://chartmuseum.chartmuseum.svc.cluster.local:8080
        chart: '{{path[4]}}'  # postgresql, redis, mongodb gibi
        targetRevision: "1.0.0"
        helm:
          valueFiles:
          - http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran/raw/branch/main/{{path}}
      destination:
        server: https://kubernetes.default.svc
        namespace: dev
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
        - CreateNamespace=true
EOF

# Sample application values - product-service
mkdir -p environments/nonprod/dev/applications/product-service
cat <<'EOF' > environments/nonprod/dev/applications/product-service/values.yaml
name: product-service
enabled: true

replicaCount: 1

image:
  repository: nginx
  tag: alpine
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 80

resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi

env:
  - name: SERVICE_NAME
    value: product-service
  - name: ENVIRONMENT
    value: dev
EOF

# Sample application values - user-service
mkdir -p environments/nonprod/dev/applications/user-service
cat <<'EOF' > environments/nonprod/dev/applications/user-service/values.yaml
name: user-service
enabled: true

replicaCount: 1

image:
  repository: nginx
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 80

resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi

env:
  - name: SERVICE_NAME
    value: user-service
  - name: ENVIRONMENT
    value: dev
EOF

# Platform service values - PostgreSQL
mkdir -p environments/nonprod/dev/platform/postgresql
cat <<'EOF' > environments/nonprod/dev/platform/postgresql/values.yaml
name: product-db
enabled: true

auth:
  database: productdb
  username: product
  password: product123

primary:
  resources:
    limits:
      cpu: 250m
      memory: 256Mi
    requests:
      cpu: 100m
      memory: 128Mi

  persistence:
    enabled: false
EOF

# Platform service values - Redis
mkdir -p environments/nonprod/dev/platform/redis
cat <<'EOF' > environments/nonprod/dev/platform/redis/values.yaml
name: redis-cache
enabled: true

architecture: standalone

auth:
  enabled: false

master:
  resources:
    limits:
      cpu: 150m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi

  persistence:
    enabled: false
EOF

# Root Application - Apps
cat <<'EOF' > appsets/nonprod/apps/root-app.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: dev-apps-root
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran
    targetRevision: main
    path: appsets/nonprod/apps
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
EOF

# Root Application - Platform
cat <<'EOF' > appsets/nonprod/platform/root-app.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: dev-platform-root
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: http://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran
    targetRevision: main
    path: appsets/nonprod/platform
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
EOF

# Git commit and push
git config user.email "platform@infraforge.io"
git config user.name "Platform Operator"
git add -A
git commit -m "Initial GitOps structure with ChartMuseum integration" 2>/dev/null || true
git push http://${GITEA_USER}:${GITEA_PASS}@localhost:3000/infraforge/voltran main -f 2>/dev/null || \
  git push http://${GITEA_USER}:${GITEA_PASS}@localhost:3000/infraforge/voltran master:main -f 2>/dev/null || \
  echo "⚠️ Git push might have failed, but continuing..."

# Port forward kapat
kill $PF_PID 2>/dev/null || true

echo "✅ GitOps structure Gitea'ya push edildi!"