// +kubebuilder:object:generate=true
// +groupName=platform.infraforge.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ApplicationClaimSpec defines the desired state of ApplicationClaim
type ApplicationClaimSpec struct {
	// Environment deployment environment (dev, qa, sandbox, staging, prod)
	Environment string `json:"environment"`

	// ClusterType cluster type (nonprod, prod) for GitOps structure
	ClusterType string `json:"clusterType"`

	// Applications multi-application support
	Applications []ApplicationSpec `json:"applications"`

	// Components platform components like databases
	Components []ComponentSpec `json:"components,omitempty"`

	// Namespace target namespace (auto-generated if empty)
	Namespace string `json:"namespace,omitempty"`

	// Owner team ownership information
	Owner OwnerSpec `json:"owner"`
}

// ApplicationSpec single application configuration
type ApplicationSpec struct {
	// Name application name
	Name string `json:"name"`

	// Chart Helm chart configuration
	Chart ChartSpec `json:"chart"`

	// Image container image configuration
	Image ImageSpec `json:"image"`

	// Replicas number of replicas
	Replicas int32 `json:"replicas,omitempty"`

	// Resources CPU/memory requirements
	Resources ResourceRequirements `json:"resources,omitempty"`

	// Ports exposed ports
	Ports []PortSpec `json:"ports,omitempty"`

	// HealthCheck health check configuration
	HealthCheck HealthCheckSpec `json:"healthCheck,omitempty"`

	// Env environment variables
	Env []EnvVar `json:"env,omitempty"`

	// Autoscaling autoscaling configuration
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// Ingress ingress configuration
	Ingress *IngressSpec `json:"ingress,omitempty"`
}

// ChartSpec defines Helm chart source
type ChartSpec struct {
	// Name chart name
	Name string `json:"name"`

	// Source chart source (embedded, github, helm-repo)
	Source string `json:"source,omitempty"`

	// Repository Helm repository URL (if external)
	Repository string `json:"repository,omitempty"`

	// Version chart version
	Version string `json:"version,omitempty"`
}

// ImageSpec defines container image configuration
type ImageSpec struct {
	// Repository image repository (e.g., "ghcr.io/infraforge/ecommerce-platform")
	Repository string `json:"repository"`

	// Tag image tag (e.g., "v1.2.3")
	Tag string `json:"tag"`

	// PullPolicy image pull policy (Always, IfNotPresent, Never)
	PullPolicy string `json:"pullPolicy,omitempty"`

	// PullSecrets image pull secrets
	PullSecrets []string `json:"pullSecrets,omitempty"`
}

// IngressSpec defines ingress configuration
type IngressSpec struct {
	// Enabled enable ingress
	Enabled bool `json:"enabled"`

	// Host ingress hostname
	Host string `json:"host,omitempty"`

	// Path ingress path
	Path string `json:"path,omitempty"`

	// TLS enable TLS
	TLS bool `json:"tls,omitempty"`

	// Annotations ingress annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ComponentSpec platform component specification
type ComponentSpec struct {
	// Type component type (postgresql, redis, rabbitmq)
	Type string `json:"type"`

	// Name instance name
	Name string `json:"name"`

	// Version component version
	Version string `json:"version,omitempty"`

	// Size configuration size (small, medium, large)
	Size string `json:"size,omitempty"`

	// Config additional configuration (supports nested YAML)
	// +kubebuilder:pruning:PreserveUnknownFields
	Config runtime.RawExtension `json:"config,omitempty"`
}

// ResourceRequirements resource requirements
type ResourceRequirements struct {
	// Requests resource requests
	Requests ResourceList `json:"requests,omitempty"`

	// Limits resource limits
	Limits ResourceList `json:"limits,omitempty"`
}

// ResourceList resource quantities
type ResourceList struct {
	// CPU in millicores (e.g., "100m")
	CPU string `json:"cpu,omitempty"`

	// Memory in bytes (e.g., "128Mi")
	Memory string `json:"memory,omitempty"`
}

// PortSpec port configuration
type PortSpec struct {
	Name     string `json:"name"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

// HealthCheckSpec health check configuration
type HealthCheckSpec struct {
	// Path HTTP path for health check
	Path string `json:"path,omitempty"`

	// Port port for health check
	Port int32 `json:"port,omitempty"`

	// InitialDelaySeconds delay before first check
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds check interval
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// EnvVar environment variable
type EnvVar struct {
	// Name variable name
	Name string `json:"name"`

	// Value variable value
	Value string `json:"value,omitempty"`

	// ValueFrom source for the variable value
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

// EnvVarSource environment variable source
type EnvVarSource struct {
	// SecretKeyRef secret reference
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`

	// ConfigMapKeyRef configmap reference
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

// SecretKeySelector secret key reference
type SecretKeySelector struct {
	// Name secret name
	Name string `json:"name"`

	// Key key in the secret
	Key string `json:"key"`
}

// ConfigMapKeySelector configmap key reference
type ConfigMapKeySelector struct {
	// Name configmap name
	Name string `json:"name"`

	// Key key in the configmap
	Key string `json:"key"`
}

// AutoscalingSpec autoscaling configuration
type AutoscalingSpec struct {
	// Enabled enable autoscaling
	Enabled bool `json:"enabled"`

	// MinReplicas minimum replicas
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas maximum replicas
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// TargetCPUUtilizationPercentage target CPU percentage
	TargetCPUUtilizationPercentage int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// TargetMemoryUtilizationPercentage target memory percentage
	TargetMemoryUtilizationPercentage int32 `json:"targetMemoryUtilizationPercentage,omitempty"`
}

// OwnerSpec ownership information
type OwnerSpec struct {
	// Team team name
	Team string `json:"team"`

	// Email contact email
	Email string `json:"email"`

	// Slack slack channel
	Slack string `json:"slack,omitempty"`
}

// ApplicationClaimStatus defines the observed state of ApplicationClaim
type ApplicationClaimStatus struct {
	// Phase current phase (Pending, Provisioning, Ready, Failed)
	Phase string `json:"phase,omitempty"`

	// Ready overall readiness status
	Ready bool `json:"ready"`

	// ApplicationsReady all applications ready
	ApplicationsReady bool `json:"applicationsReady"`

	// ComponentsReady all components ready
	ComponentsReady bool `json:"componentsReady"`

	// Applications application statuses
	Applications []ApplicationStatus `json:"applications,omitempty"`

	// Components component statuses
	Components []ComponentStatus `json:"components,omitempty"`

	// Conditions detailed conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdated last update timestamp
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// ApplicationStatus application deployment status
type ApplicationStatus struct {
	Name              string   `json:"name"`
	Ready             bool     `json:"ready"`
	Version           string   `json:"version"`
	Replicas          int32    `json:"replicas"`
	AvailableReplicas int32    `json:"availableReplicas"`
	Endpoints         []string `json:"endpoints,omitempty"`
}

// ComponentStatus component provision status
type ComponentStatus struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Ready            bool   `json:"ready"`
	ConnectionString string `json:"connectionString,omitempty"`
	SecretName       string `json:"secretName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="ClusterType",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ApplicationClaim is the Schema for the applicationclaims API
type ApplicationClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationClaimSpec   `json:"spec,omitempty"`
	Status ApplicationClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationClaimList contains a list of ApplicationClaim
type ApplicationClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationClaim `json:"items"`
}
