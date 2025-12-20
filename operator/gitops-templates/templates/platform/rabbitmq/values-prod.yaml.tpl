# RabbitMQ Helm Chart Values - Production Environment
# Template for {{ .Environment }} environment

global:
  imageRegistry: docker.io

auth:
  username: "{{ .RabbitMQUser | default "admin" }}"
  password: "{{ .RabbitMQPassword }}"
  erlangCookie: "{{ .ErlangCookie }}"

image:
  registry: docker.io
  repository: bitnami/rabbitmq
  tag: "3.12.10"
  pullPolicy: IfNotPresent

replicaCount: 3

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
  accessMode: ReadWriteOnce

service:
  type: ClusterIP
  ports:
    amqp: 5672
    dist: 25672
    manager: 15672
    epmd: 4369
    metrics: 9419

ingress:
  enabled: false

clustering:
  enabled: true
  addressType: hostname
  rebalance: true
  forceBoot: false

loadDefinition:
  enabled: true
  existingSecret: ""

extraConfiguration: |-
  default_user = {{ .RabbitMQUser | default "admin" }}
  default_pass = {{ .RabbitMQPassword }}
  default_user_tags.administrator = true
  default_permissions.configure = .*
  default_permissions.read = .*
  default_permissions.write = .*

  ## Clustering
  cluster_partition_handling = autoheal
  cluster_formation.peer_discovery_backend = rabbit_peer_discovery_k8s
  cluster_formation.k8s.host = kubernetes.default.svc.cluster.local
  cluster_formation.k8s.address_type = hostname
  cluster_formation.node_cleanup.interval = 30
  cluster_formation.node_cleanup.only_log_warning = true
  cluster_keepalive_interval = 10000

  ## Networking
  tcp_listen_options.backlog = 1024
  tcp_listen_options.nodelay = true
  tcp_listen_options.linger.on = true
  tcp_listen_options.linger.timeout = 0
  tcp_listen_options.sndbuf = 196608
  tcp_listen_options.recbuf = 196608

  ## Memory and Performance
  vm_memory_high_watermark.relative = 0.7
  vm_memory_high_watermark_paging_ratio = 0.8
  total_memory_available_override_value = 2GB

  ## Disk
  disk_free_limit.absolute = 5GB

  ## Logging
  log.console = true
  log.console.level = info
  log.file.level = info

  ## Message TTL
  message_ttl = 7200000

  ## Mirroring (HA queues)
  cluster_formation.target_cluster_size_hint = 3
  queue_master_locator = min-masters

  ## Production optimizations
  collect_statistics_interval = 30000
  management.rates_mode = detailed

  ## Connection limits
  channel_max = 2048
  max_message_size = 134217728

plugins: "rabbitmq_management rabbitmq_peer_discovery_k8s rabbitmq_prometheus rabbitmq_shovel rabbitmq_shovel_management"

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
                - rabbitmq
        topologyKey: kubernetes.io/hostname

podSecurityContext:
  enabled: true
  fsGroup: 1001
  runAsUser: 1001

containerSecurityContext:
  enabled: true
  runAsUser: 1001
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL

metrics:
  enabled: true
  serviceMonitor:
    enabled: {{ .PrometheusEnabled | default true }}
    namespace: "{{ .Environment }}"
    interval: 30s
    scrapeTimeout: 10s

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
              rabbitmq-client: "true"

serviceAccount:
  create: true
  automountServiceAccountToken: true

rbac:
  create: true

# Production specific configurations
memoryHighWatermark:
  enabled: true
  type: "relative"
  value: 0.7

ulimitNofiles: 65535

podManagementPolicy: OrderedReady

updateStrategy:
  type: RollingUpdate

podDisruptionBudget:
  minAvailable: 2

livenessProbe:
  enabled: true
  initialDelaySeconds: 120
  timeoutSeconds: 20
  periodSeconds: 30
  failureThreshold: 3
  successThreshold: 1

readinessProbe:
  enabled: true
  initialDelaySeconds: 10
  timeoutSeconds: 20
  periodSeconds: 30
  failureThreshold: 3
  successThreshold: 1

startupProbe:
  enabled: true
  initialDelaySeconds: 0
  timeoutSeconds: 10
  periodSeconds: 10
  failureThreshold: 30
  successThreshold: 1