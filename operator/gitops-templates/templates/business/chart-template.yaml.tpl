# Chart.yaml Template for Microservices
apiVersion: v2
name: {{ .ServiceName }}
description: Helm chart for {{ .ServiceName }} microservice
type: application
version: {{ .ChartVersion | default "0.1.0" }}
appVersion: {{ .AppVersion | default "1.0.0" }}
keywords:
  - microservice
  - {{ .ServiceName }}
  - {{ .Environment }}
maintainers:
  - name: Platform Team
    email: platform@{{ .Domain }}
sources:
  - {{ .GitopsRepoURL }}
home: https://{{ .ServiceName }}.{{ .Domain }}