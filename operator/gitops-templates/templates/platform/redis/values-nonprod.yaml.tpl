# Redis Helm Chart Values - NonProd Environment
# Template for {{ .Environment }} environment

global:
  redis:
    password: "{{ .RedisPassword }}"

image:
  registry: docker.io
  repository: bitnami/redis
  tag: "7.2.3"
  pullPolicy: IfNotPresent

architecture: standalone

auth:
  enabled: true
  sentinel: true
  password: "{{ .RedisPassword }}"

master:
  count: 1

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "250m"
      memory: "256Mi"

  persistence:
    enabled: true
    size: "8Gi"
    storageClass: "gp3"
    accessModes:
      - ReadWriteOnce

  nodeSelector:
    workload: "general"

  tolerations: []

  affinity: {}

  service:
    type: ClusterIP
    port: 6379

replica:
  replicaCount: 0

metrics:
  enabled: true
  image:
    registry: docker.io
    repository: bitnami/redis-exporter
    tag: "1.55.0"

  resources:
    requests:
      cpu: "50m"
      memory: "64Mi"
    limits:
      cpu: "100m"
      memory: "128Mi"

  serviceMonitor:
    enabled: {{ .PrometheusEnabled | default false }}
    namespace: "{{ .Environment }}"

volumePermissions:
  enabled: true
  image:
    registry: docker.io
    repository: bitnami/os-shell
    tag: "11"

networkPolicy:
  enabled: false
  allowExternal: true

serviceAccount:
  create: true
  automountServiceAccountToken: false

rbac:
  create: false

podSecurityContext:
  enabled: true
  fsGroup: 1001
  runAsUser: 1001

containerSecurityContext:
  enabled: true
  runAsUser: 1001
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: false
  capabilities:
    drop:
      - ALL

# Redis Configuration for Development/QA
commonConfiguration: |-
  # Enable AOF persistence
  appendonly yes
  appendfsync everysec

  # Save snapshots
  save 900 1
  save 300 10
  save 60 10000

  # Max memory policy
  maxmemory-policy allkeys-lru

  # Disable dangerous commands
  rename-command FLUSHDB ""
  rename-command FLUSHALL ""
  rename-command CONFIG ""

  # Connection settings
  timeout 300
  tcp-keepalive 60
  tcp-backlog 511

  # Slow log
  slowlog-log-slower-than 10000
  slowlog-max-len 128