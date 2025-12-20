# Redis Helm Chart Values - Production Environment
# Template for {{ .Environment }} environment

global:
  redis:
    password: "{{ .RedisPassword }}"

image:
  registry: docker.io
  repository: bitnami/redis
  tag: "7.2.3"
  pullPolicy: IfNotPresent

architecture: replication

auth:
  enabled: true
  sentinel: true
  password: "{{ .RedisPassword }}"

master:
  count: 1

  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "1"
      memory: "2Gi"

  persistence:
    enabled: true
    size: "20Gi"
    storageClass: "gp3-retain"
    accessModes:
      - ReadWriteOnce

  nodeSelector:
    workload: "database"

  tolerations:
    - key: "database"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"

  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - redis
              - key: app.kubernetes.io/component
                operator: In
                values:
                  - master
          topologyKey: kubernetes.io/hostname

  service:
    type: ClusterIP
    port: 6379

replica:
  replicaCount: 2

  resources:
    requests:
      cpu: "250m"
      memory: "512Mi"
    limits:
      cpu: "500m"
      memory: "1Gi"

  persistence:
    enabled: true
    size: "20Gi"
    storageClass: "gp3-retain"

  nodeSelector:
    workload: "database"

  tolerations:
    - key: "database"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"

  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchExpressions:
                - key: app.kubernetes.io/name
                  operator: In
                  values:
                    - redis
            topologyKey: kubernetes.io/hostname

sentinel:
  enabled: true
  image:
    registry: docker.io
    repository: bitnami/redis-sentinel
    tag: "7.2.3"

  masterSet: mymaster
  quorum: 2

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"

metrics:
  enabled: true
  image:
    registry: docker.io
    repository: bitnami/redis-exporter
    tag: "1.55.0"

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"

  serviceMonitor:
    enabled: {{ .PrometheusEnabled | default true }}
    namespace: "{{ .Environment }}"

volumePermissions:
  enabled: true
  image:
    registry: docker.io
    repository: bitnami/os-shell
    tag: "11"

networkPolicy:
  enabled: true
  allowExternal: false

  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: "{{ .Environment }}"
        - podSelector:
            matchLabels:
              redis-client: "true"

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

# Redis Configuration for Production
commonConfiguration: |-
  # Enable AOF persistence
  appendonly yes
  appendfsync everysec

  # Save snapshots more frequently in production
  save 900 1
  save 300 10
  save 60 10000

  # Max memory policy
  maxmemory-policy allkeys-lru

  # Disable dangerous commands
  rename-command FLUSHDB ""
  rename-command FLUSHALL ""
  rename-command CONFIG ""
  rename-command KEYS ""
  rename-command DEBUG ""

  # Connection settings
  timeout 300
  tcp-keepalive 60
  tcp-backlog 511

  # Slow log
  slowlog-log-slower-than 10000
  slowlog-max-len 256

  # Client output buffer limits
  client-output-buffer-limit normal 0 0 0
  client-output-buffer-limit replica 256mb 64mb 60
  client-output-buffer-limit pubsub 32mb 8mb 60

  # Production optimizations
  hz 100
  databases 16

  # Replication settings
  repl-diskless-sync yes
  repl-diskless-sync-delay 5
  repl-ping-replica-period 10
  repl-timeout 60
  repl-backlog-size 32mb
  repl-backlog-ttl 3600

  # Protection mode
  protected-mode yes

  # Max clients
  maxclients 10000