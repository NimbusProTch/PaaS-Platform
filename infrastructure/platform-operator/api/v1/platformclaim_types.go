// +kubebuilder:object:generate=true
// +groupName=platform.infraforge.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PlatformClaimSpec defines the desired state of PlatformClaim
type PlatformClaimSpec struct {
	// Environment deployment environment (dev, qa, sandbox, staging, prod)
	Environment string `json:"environment"`

	// ClusterType cluster type (nonprod, prod)
	ClusterType string `json:"clusterType"`

	// Services platform services to deploy
	Services []PlatformServiceSpec `json:"services"`

	// Namespace target namespace (auto-generated if empty)
	Namespace string `json:"namespace,omitempty"`

	// Owner team ownership information
	Owner OwnerSpec `json:"owner"`
}

// PlatformServiceSpec defines a platform service configuration
type PlatformServiceSpec struct {
	// Name service name (e.g., "postgres", "redis", "rabbitmq")
	Name string `json:"name"`

	// Type service type (postgresql, redis, rabbitmq, mongodb, mysql, kafka, elasticsearch)
	Type string `json:"type"`

	// Version service version (optional)
	Version string `json:"version,omitempty"`

	// Chart Helm chart configuration
	Chart ChartSpec `json:"chart"`

	// Values custom values for the service
	// +kubebuilder:pruning:PreserveUnknownFields
	Values runtime.RawExtension `json:"values,omitempty"`

	// Size configuration size (small, medium, large)
	Size string `json:"size,omitempty"`

	// HighAvailability enable HA configuration
	HighAvailability bool `json:"highAvailability,omitempty"`

	// Backup enable backup configuration
	Backup *BackupSpec `json:"backup,omitempty"`

	// Monitoring enable monitoring
	Monitoring bool `json:"monitoring,omitempty"`
}

// BackupSpec defines backup configuration
type BackupSpec struct {
	// Enabled enable backups
	Enabled bool `json:"enabled"`

	// Schedule cron schedule for backups
	Schedule string `json:"schedule,omitempty"`

	// Retention retention period in days
	Retention int `json:"retention,omitempty"`

	// StorageClass storage class for backup volumes
	StorageClass string `json:"storageClass,omitempty"`
}

// PlatformClaimStatus defines the observed state of PlatformClaim
type PlatformClaimStatus struct {
	// Phase current phase (Pending, Provisioning, Ready, Failed)
	Phase string `json:"phase,omitempty"`

	// Ready overall readiness status
	Ready bool `json:"ready"`

	// ServicesReady all services ready
	ServicesReady bool `json:"servicesReady"`

	// Services service statuses
	Services []PlatformServiceStatus `json:"services,omitempty"`

	// Conditions detailed conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdated last update timestamp
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Message provides additional status information
	Message string `json:"message,omitempty"`
}

// PlatformServiceStatus defines the status of a platform service
type PlatformServiceStatus struct {
	// Name service name
	Name string `json:"name"`

	// Type service type
	Type string `json:"type"`

	// Ready service ready status
	Ready bool `json:"ready"`

	// Version deployed version
	Version string `json:"version,omitempty"`

	// Endpoint service endpoint
	Endpoint string `json:"endpoint,omitempty"`

	// SecretName secret containing credentials
	SecretName string `json:"secretName,omitempty"`

	// Message additional status message
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="ClusterType",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PlatformClaim is the Schema for the platformclaims API
type PlatformClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformClaimSpec   `json:"spec,omitempty"`
	Status PlatformClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PlatformClaimList contains a list of PlatformClaim
type PlatformClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlatformClaim `json:"items"`
}
