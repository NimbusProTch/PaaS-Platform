# PostgreSQL Helm Chart Values - NonProd Environment
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

architecture: standalone

primary:
  resources:
    requests:
      cpu: "250m"
      memory: "256Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"

  persistence:
    enabled: true
    size: "10Gi"
    storageClass: "gp3"
    accessModes:
      - ReadWriteOnce

  nodeSelector:
    workload: "general"

  tolerations: []

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
      cpu: "50m"
      memory: "64Mi"
    limits:
      cpu: "100m"
      memory: "128Mi"

  serviceMonitor:
    enabled: {{ .PrometheusEnabled | default false }}
    namespace: "{{ .Environment }}"

backup:
  enabled: false

networkPolicy:
  enabled: false

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

# Development/QA specific settings
audit:
  logConnections: false
  logDisconnections: false

postgresql:
  maxConnections: 100
  sharedBuffers: "128MB"
  effectiveCacheSize: "512MB"
  maintenanceWorkMem: "64MB"
  walBuffers: "4MB"
  defaultStatisticsTarget: 100
  randomPageCost: 4
  effectiveIoConcurrency: 2
  workMem: "4MB"
  hugePages: off