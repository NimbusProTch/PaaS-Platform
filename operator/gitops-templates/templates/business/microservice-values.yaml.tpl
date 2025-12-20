# Microservice Helm Chart Values Template
# Used for {{ .ServiceName }} in {{ .Environment }} environment

replicaCount: {{ .ReplicaCount | default 2 }}

image:
  repository: {{ .ImageRepository }}
  tag: {{ .ImageTag | default "latest" }}
  pullPolicy: {{ .ImagePullPolicy | default "IfNotPresent" }}

nameOverride: "{{ .ServiceName }}"
fullnameOverride: "{{ .ServiceName }}"

serviceAccount:
  create: true
  annotations: {}
  name: "{{ .ServiceName }}"

podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "{{ .MetricsPort | default "8080" }}"
  prometheus.io/path: "{{ .MetricsPath | default "/metrics" }}"

podSecurityContext:
  fsGroup: 2000
  runAsNonRoot: true
  runAsUser: 1000

securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 1000

service:
  type: ClusterIP
  port: {{ .ServicePort | default 8080 }}
  targetPort: {{ .ServiceTargetPort | default 8080 }}
  annotations: {}

ingress:
  enabled: {{ .IngressEnabled | default false }}
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-{{ .Environment }}"
    nginx.ingress.kubernetes.io/rewrite-target: /
  hosts:
    - host: {{ .ServiceName }}.{{ .Domain }}
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: {{ .ServiceName }}-tls
      hosts:
        - {{ .ServiceName }}.{{ .Domain }}

resources:
  {{- if eq .Environment "prod" }}
  limits:
    cpu: {{ .ResourceLimitsCPU | default "500m" }}
    memory: {{ .ResourceLimitsMemory | default "512Mi" }}
  requests:
    cpu: {{ .ResourceRequestsCPU | default "250m" }}
    memory: {{ .ResourceRequestsMemory | default "256Mi" }}
  {{- else }}
  limits:
    cpu: {{ .ResourceLimitsCPU | default "200m" }}
    memory: {{ .ResourceLimitsMemory | default "256Mi" }}
  requests:
    cpu: {{ .ResourceRequestsCPU | default "100m" }}
    memory: {{ .ResourceRequestsMemory | default "128Mi" }}
  {{- end }}

autoscaling:
  enabled: {{ .AutoscalingEnabled | default true }}
  minReplicas: {{ .MinReplicas | default 2 }}
  maxReplicas: {{ .MaxReplicas | default 10 }}
  targetCPUUtilizationPercentage: {{ .TargetCPU | default 70 }}
  targetMemoryUtilizationPercentage: {{ .TargetMemory | default 80 }}

nodeSelector:
  {{- if eq .Environment "prod" }}
  workload: "production"
  {{- else }}
  workload: "general"
  {{- end }}

tolerations:
  {{- if eq .Environment "prod" }}
  - key: "production"
    operator: "Equal"
    value: "true"
    effect: "NoSchedule"
  {{- end }}

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
                  - {{ .ServiceName }}
          topologyKey: kubernetes.io/hostname

livenessProbe:
  httpGet:
    path: {{ .HealthPath | default "/health" }}
    port: http
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: {{ .ReadinessPath | default "/ready" }}
    port: http
  initialDelaySeconds: 10
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3

env:
  - name: ENVIRONMENT
    value: "{{ .Environment }}"
  - name: SERVICE_NAME
    value: "{{ .ServiceName }}"
  - name: LOG_LEVEL
    value: "{{ .LogLevel | default "info" }}"
  {{- if .PostgresEnabled }}
  - name: DATABASE_HOST
    value: "postgresql-{{ .Environment }}"
  - name: DATABASE_PORT
    value: "5432"
  - name: DATABASE_NAME
    value: "{{ .DatabaseName | default .ServiceName }}"
  - name: DATABASE_USER
    valueFrom:
      secretKeyRef:
        name: "postgresql-{{ .Environment }}"
        key: username
  - name: DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: "postgresql-{{ .Environment }}"
        key: password
  {{- end }}
  {{- if .RedisEnabled }}
  - name: REDIS_HOST
    value: "redis-{{ .Environment }}-master"
  - name: REDIS_PORT
    value: "6379"
  - name: REDIS_PASSWORD
    valueFrom:
      secretKeyRef:
        name: "redis-{{ .Environment }}"
        key: redis-password
  {{- end }}
  {{- if .RabbitMQEnabled }}
  - name: RABBITMQ_HOST
    value: "rabbitmq-{{ .Environment }}"
  - name: RABBITMQ_PORT
    value: "5672"
  - name: RABBITMQ_USERNAME
    valueFrom:
      secretKeyRef:
        name: "rabbitmq-{{ .Environment }}"
        key: username
  - name: RABBITMQ_PASSWORD
    valueFrom:
      secretKeyRef:
        name: "rabbitmq-{{ .Environment }}"
        key: password
  {{- end }}
  {{- range .ExtraEnv }}
  - name: {{ .Name }}
    value: "{{ .Value }}"
  {{- end }}

configMap:
  enabled: {{ .ConfigMapEnabled | default false }}
  data:
    {{- range $key, $value := .ConfigMapData }}
    {{ $key }}: |
{{ $value | indent 6 }}
    {{- end }}

secrets:
  enabled: {{ .SecretsEnabled | default false }}
  data:
    {{- range $key, $value := .SecretsData }}
    {{ $key }}: {{ $value | b64enc }}
    {{- end }}

serviceMonitor:
  enabled: {{ .PrometheusEnabled | default false }}
  namespace: "{{ .Environment }}"
  interval: 30s
  path: {{ .MetricsPath | default "/metrics" }}

podDisruptionBudget:
  enabled: {{ .PDBEnabled | default true }}
  minAvailable: 1

networkPolicy:
  enabled: {{ .NetworkPolicyEnabled | default false }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: "{{ .Environment }}"
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              name: "{{ .Environment }}"
    - to:
        - namespaceSelector:
            matchLabels:
              name: kube-system
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP