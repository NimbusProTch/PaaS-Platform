# PostgreSQL Helm Chart Values - Production Environment
# Template for {{ .Environment }} environment

global:
  postgresql:
    auth:
      database: "{{ .DatabaseName | default "appdb" }}"
      username: "{{ .DatabaseUser | default "appuser" }}"
      password: "{{ .DatabasePassword }}"
      postgresPassword: "{{ .PostgresPassword }}"

image:
  registry: docker.io
  repository: bitnami/postgresql
  tag: "15.4.0"
  pullPolicy: IfNotPresent

architecture: replication

postgresql:
  replicaCount: 2

primary:
  resources:
    requests:
      cpu: "1"
      memory: "2Gi"
    limits:
      cpu: "2"
      memory: "4Gi"

  persistence:
    enabled: true
    size: "50Gi"
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
                  - postgresql
          topologyKey: kubernetes.io/hostname

readReplicas:
  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "1"
      memory: "2Gi"

  persistence:
    enabled: true
    size: "50Gi"
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
                    - postgresql
            topologyKey: kubernetes.io/hostname

metrics:
  enabled: true
  image:
    registry: docker.io
    repository: bitnami/postgres-exporter
    tag: "0.13.2"

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

backup:
  enabled: true
  cronjob:
    schedule: "0 2 * * *"
    concurrencyPolicy: Forbid
    successfulJobsHistoryLimit: 3
    failedJobsHistoryLimit: 3

  storage:
    existingClaim: ""
    size: "100Gi"
    storageClass: "gp3-retain"

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
              postgresql-client: "true"

serviceAccount:
  create: true
  automountServiceAccountToken: false

rbac:
  create: false

podSecurityContext:
  enabled: true
  fsGroup: 1001
  runAsNonRoot: true

containerSecurityContext:
  enabled: true
  runAsUser: 1001
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL

# Production optimized settings
audit:
  logConnections: true
  logDisconnections: true
  logHostname: true
  logLinePrefix: '%t [%p]: [%l-1] user=%u,db=%d,app=%a,client=%h '
  pgAuditLog: 'all'

postgresql:
  maxConnections: 200
  sharedBuffers: "1GB"
  effectiveCacheSize: "3GB"
  maintenanceWorkMem: "256MB"
  walBuffers: "16MB"
  defaultStatisticsTarget: 100
  randomPageCost: 1.1
  effectiveIoConcurrency: 200
  workMem: "16MB"
  hugePages: try

  # Replication settings
  walLevel: replica
  maxWalSenders: 10
  walKeepSegments: 64
  maxReplicationSlots: 10

  # Checkpoint settings
  checkpointCompletionTarget: 0.9
  checkpointTimeout: "15min"

  # WAL settings
  walCompression: on
  archiveMode: on
  archiveCommand: 'test ! -f /backup/%f && cp %p /backup/%f'

  # Connection pooling
  maxPreparedTransactions: 100