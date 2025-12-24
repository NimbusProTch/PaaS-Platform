  Gitea Organization: platform
  │
  ├── 📦 charts/                    (Application Helm Charts)
  │   ├── ecommerce-platform/
  │   ├── user-service/
  │   └── product-service/
  │
  ├── 📦 platform-charts/           (Platform Services - Common)
  │   ├── postgres/
  │   │   ├── Chart.yaml
  │   │   ├── values.yaml
  │   │   └── templates/
  │   ├── rabbitmq/
  │   ├── redis/
  │   └── kafka/
  │
  └── 📦 voltran/                   (GitOps Config Repo)
      ├── root-apps/
      │   ├── nonprod-apps-rootapp.yaml       🔥 Application apps için
      │   ├── nonprod-platform-rootapp.yaml   🔥 Platform services için
      │   ├── prod-apps-rootapp.yaml
      │   └── prod-platform-rootapp.yaml
      │
      ├── appsets/
      │   ├── nonprod/
      │   │   ├── apps/                       🔥 YENİ
      │   │   │   ├── dev-appset.yaml         (Operator oluşturur)
      │   │   │   ├── qa-appset.yaml
      │   │   │   └── sandbox-appset.yaml
      │   │   └── platform/                   🔥 YENİ
      │   │       ├── dev-platform-appset.yaml     (Operator oluşturur)
      │   │       ├── qa-platform-appset.yaml
      │   │       └── sandbox-platform-appset.yaml
      │   └── prod/
      │       ├── apps/
      │       │   ├── prod-appset.yaml
      │       │   └── stage-appset.yaml
      │       └── platform/
      │           ├── prod-platform-appset.yaml
      │           └── stage-platform-appset.yaml
      │
      └── environments/
          ├── nonprod/
          │   ├── dev/
          │   │   ├── applications/           🔥 Business Apps
          │   │   │   ├── ecommerce-platform/
          │   │   │   │   └── values.yaml     (Operator: ApplicationClaim'den)
          │   │   │   ├── user-service/
          │   │   │   │   └── values.yaml
          │   │   │   └── order-service/
          │   │   │       └── values.yaml
          │   │   └── platform/               🔥 Platform Services
          │   │       ├── postgres/
          │   │       │   └── values.yaml     (Operator: PlatformClaim'den)
          │   │       ├── rabbitmq/
          │   │       │   └── values.yaml
          │   │       ├── redis/
          │   │       │   └── values.yaml
          │   │       └── kafka/
          │   │           └── values.yaml
          │   ├── qa/
          │   │   ├── applications/
          │   │   └── platform/
          │   └── sandbox/
          │       ├── applications/
          │       └── platform/
          │
          └── prod/
              ├── prod/
              │   ├── applications/
              │   └── platform/
              └── stage/
                  ├── applications/
                  └── platform/