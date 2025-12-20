# RabbitMQ Helm Chart Values - NonProd Environment
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

replicaCount: 1

resources:
  requests:
    cpu: "250m"
    memory: "256Mi"
  limits:
    cpu: "500m"
    memory: "512Mi"

persistence:
  enabled: true
  size: "8Gi"
  storageClass: "gp3"
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
  enabled: false

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

  ## Networking
  tcp_listen_options.backlog = 511
  tcp_listen_options.nodelay = true
  tcp_listen_options.linger.on = true
  tcp_listen_options.linger.timeout = 0
  tcp_listen_options.sndbuf = 32768
  tcp_listen_options.recbuf = 32768

  ## Memory
  vm_memory_high_watermark.relative = 0.6
  vm_memory_high_watermark_paging_ratio = 0.75

  ## Disk
  disk_free_limit.absolute = 2GB

  ## Logging
  log.console = true
  log.console.level = info

  ## Message TTL
  message_ttl = 3600000

plugins: "rabbitmq_management rabbitmq_peer_discovery_k8s rabbitmq_prometheus"

nodeSelector:
  workload: "general"

tolerations: []

affinity: {}

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
    enabled: {{ .PrometheusEnabled | default false }}
    namespace: "{{ .Environment }}"

networkPolicy:
  enabled: false
  allowExternal: true

serviceAccount:
  create: true
  automountServiceAccountToken: false

rbac:
  create: true

# Development specific configurations
memoryHighWatermark:
  enabled: true
  type: "relative"
  value: 0.6

ulimitNofiles: 65535