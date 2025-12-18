# Kratix Platform Installation
resource "helm_release" "cert_manager" {
  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  version          = "v1.13.3"
  namespace        = "cert-manager"
  create_namespace = true

  set {
    name  = "installCRDs"
    value = "true"
  }

  set {
    name  = "global.leaderElection.namespace"
    value = "cert-manager"
  }

  depends_on = [module.eks]
}

# Kratix Installation
resource "kubectl_manifest" "kratix_platform" {
  yaml_body = file("${path.module}/manifests/kratix-platform.yaml")

  depends_on = [
    helm_release.cert_manager,
    module.eks
  ]
}

# Kratix State Store (MinIO or S3)
resource "helm_release" "minio" {
  count = var.enable_kratix_minio ? 1 : 0

  name             = "minio"
  repository       = "https://charts.min.io"
  chart            = "minio"
  version          = "5.0.14"
  namespace        = "kratix-platform-system"
  create_namespace = true

  values = [
    <<-EOT
    mode: standalone

    persistence:
      enabled: true
      size: 20Gi
      storageClass: gp3

    service:
      type: ClusterIP

    consoleService:
      type: LoadBalancer
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"

    rootUser: admin
    rootPassword: ${random_password.minio_password[0].result}

    buckets:
      - name: kratix
        policy: none
        purge: false

    resources:
      requests:
        memory: 512Mi
        cpu: 250m
      limits:
        memory: 1Gi
        cpu: 500m
    EOT
  ]

  depends_on = [kubectl_manifest.kratix_platform]
}

resource "random_password" "minio_password" {
  count   = var.enable_kratix_minio ? 1 : 0
  length  = 16
  special = false
}

# Kratix Promises
resource "kubectl_manifest" "kratix_postgresql_promise" {
  yaml_body = <<-YAML
apiVersion: platform.kratix.io/v1alpha1
kind: Promise
metadata:
  name: postgresql
  namespace: kratix-platform-system
spec:
  api:
    apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: postgresqls.marketplace.kratix.io
    spec:
      group: marketplace.kratix.io
      names:
        kind: PostgreSQL
        plural: postgresqls
        singular: postgresql
      scope: Namespaced
      versions:
      - name: v1alpha1
        schema:
          openAPIV3Schema:
            properties:
              spec:
                properties:
                  dbName:
                    type: string
                  size:
                    type: string
                    enum: ["small", "medium", "large"]
                  version:
                    type: string
                    default: "14"
                type: object
            type: object
        served: true
        storage: true

  destinationSelectors:
  - matchLabels:
      environment: dev

  workflows:
    resource:
      configure:
      - apiVersion: platform.kratix.io/v1alpha1
        kind: Pipeline
        metadata:
          name: postgresql-configure
        spec:
          containers:
          - name: create-resources
            image: alpine/k8s:1.28.4
            command:
            - sh
            - -c
            - |
              cat <<EOF | kubectl apply -f -
              apiVersion: apps/v1
              kind: StatefulSet
              metadata:
                name: postgres-$(RESOURCE_NAME)
                namespace: $(RESOURCE_NAMESPACE)
              spec:
                replicas: 1
                selector:
                  matchLabels:
                    app: postgres-$(RESOURCE_NAME)
                template:
                  metadata:
                    labels:
                      app: postgres-$(RESOURCE_NAME)
                  spec:
                    containers:
                    - name: postgres
                      image: postgres:$(RESOURCE_SPEC_VERSION)
                      env:
                      - name: POSTGRES_DB
                        value: $(RESOURCE_SPEC_DBNAME)
                      - name: POSTGRES_PASSWORD
                        value: postgres123
                      volumeMounts:
                      - name: data
                        mountPath: /var/lib/postgresql/data
                volumeClaimTemplates:
                - metadata:
                    name: data
                  spec:
                    accessModes: ["ReadWriteOnce"]
                    storageClassName: gp3
                    resources:
                      requests:
                        storage: 10Gi
              EOF
  YAML

  depends_on = [kubectl_manifest.kratix_platform]
}

# Kratix Redis Promise
resource "kubectl_manifest" "kratix_redis_promise" {
  yaml_body = <<-YAML
apiVersion: platform.kratix.io/v1alpha1
kind: Promise
metadata:
  name: redis
  namespace: kratix-platform-system
spec:
  api:
    apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: redis.marketplace.kratix.io
    spec:
      group: marketplace.kratix.io
      names:
        kind: Redis
        plural: redis
        singular: redis
      scope: Namespaced
      versions:
      - name: v1alpha1
        schema:
          openAPIV3Schema:
            properties:
              spec:
                properties:
                  size:
                    type: string
                    enum: ["small", "medium", "large"]
                  persistence:
                    type: boolean
                    default: true
                type: object
            type: object
        served: true
        storage: true

  workflows:
    resource:
      configure:
      - apiVersion: platform.kratix.io/v1alpha1
        kind: Pipeline
        metadata:
          name: redis-configure
        spec:
          containers:
          - name: create-resources
            image: alpine/k8s:1.28.4
            command:
            - sh
            - -c
            - |
              cat <<EOF | kubectl apply -f -
              apiVersion: apps/v1
              kind: Deployment
              metadata:
                name: redis-$(RESOURCE_NAME)
                namespace: $(RESOURCE_NAMESPACE)
              spec:
                replicas: 1
                selector:
                  matchLabels:
                    app: redis-$(RESOURCE_NAME)
                template:
                  metadata:
                    labels:
                      app: redis-$(RESOURCE_NAME)
                  spec:
                    containers:
                    - name: redis
                      image: redis:7-alpine
                      ports:
                      - containerPort: 6379
              ---
              apiVersion: v1
              kind: Service
              metadata:
                name: redis-$(RESOURCE_NAME)
                namespace: $(RESOURCE_NAMESPACE)
              spec:
                selector:
                  app: redis-$(RESOURCE_NAME)
                ports:
                - port: 6379
                  targetPort: 6379
              EOF
  YAML

  depends_on = [kubectl_manifest.kratix_platform]
}

# Output Kratix details
output "kratix_minio_console_url" {
  value = var.enable_kratix_minio ? "Check LoadBalancer IP for MinIO console" : "MinIO not enabled"
}

output "kratix_minio_password" {
  value     = var.enable_kratix_minio ? random_password.minio_password[0].result : "N/A"
  sensitive = true
}