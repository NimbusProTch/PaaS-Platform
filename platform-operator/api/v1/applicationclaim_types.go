package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationClaimSpec defines the desired state of ApplicationClaim
type ApplicationClaimSpec struct {
	// Environment specifies deployment environment
	Environment string `json:"environment"`

	// Applications list for multi-application deployment
	Applications []ApplicationSpec `json:"applications"`

	// PlatformComponents required by applications
	PlatformComponents []PlatformComponentRef `json:"platformComponents,omitempty"`

	// Namespace override (default: generated from claim name)
	Namespace string `json:"namespace,omitempty"`

	// Owner information for tracking
	Owner OwnerSpec `json:"owner"`

	// GitHubIntegration for source control
	GitHubIntegration GitHubSpec `json:"githubIntegration,omitempty"`
}

// ApplicationSpec defines a single application deployment
type ApplicationSpec struct {
	// Name of the application
	Name string `json:"name"`

	// GitHubRelease version to deploy
	Version string `json:"version"`

	// Repository GitHub repository (org/repo format)
	Repository string `json:"repository"`

	// Replicas count (default based on environment)
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources CPU and memory requests/limits
	Resources ResourceSpec `json:"resources,omitempty"`

	// ConfigOverrides for application-specific settings
	ConfigOverrides map[string]string `json:"configOverrides,omitempty"`

	// Dependencies on other applications or components
	Dependencies []string `json:"dependencies,omitempty"`

	// Ports exposed by the application
	Ports []PortSpec `json:"ports,omitempty"`

	// HealthCheck endpoints
	HealthCheck HealthCheckSpec `json:"healthCheck,omitempty"`
}

// PlatformComponentRef references a platform component
type PlatformComponentRef struct {
	// Type of component (postgres, redis, rabbitmq, etc.)
	Type string `json:"type"`

	// Name for this instance
	Name string `json:"name"`

	// Version of the component
	Version string `json:"version,omitempty"`

	// Template GitHub template to use (prod/nonprod)
	Template string `json:"template,omitempty"`

	// Size configuration (small, medium, large)
	Size string `json:"size,omitempty"`

	// CustomConfig for additional settings
	CustomConfig map[string]interface{} `json:"customConfig,omitempty"`
}

// ResourceSpec defines resource requirements
type ResourceSpec struct {
	// CPU request and limit
	CPU ResourceQuantity `json:"cpu,omitempty"`

	// Memory request and limit
	Memory ResourceQuantity `json:"memory,omitempty"`

	// Storage requirements
	Storage ResourceQuantity `json:"storage,omitempty"`
}

// ResourceQuantity specifies request and limit
type ResourceQuantity struct {
	Request string `json:"request,omitempty"`
	Limit   string `json:"limit,omitempty"`
}

// PortSpec defines application port configuration
type PortSpec struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort int32  `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// HealthCheckSpec defines health check configuration
type HealthCheckSpec struct {
	LivenessProbe  ProbeSpec `json:"livenessProbe,omitempty"`
	ReadinessProbe ProbeSpec `json:"readinessProbe,omitempty"`
	StartupProbe   ProbeSpec `json:"startupProbe,omitempty"`
}

// ProbeSpec defines probe configuration
type ProbeSpec struct {
	Path                string `json:"path,omitempty"`
	Port                int32  `json:"port,omitempty"`
	InitialDelaySeconds int32  `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int32  `json:"periodSeconds,omitempty"`
	TimeoutSeconds      int32  `json:"timeoutSeconds,omitempty"`
	FailureThreshold    int32  `json:"failureThreshold,omitempty"`
}

// OwnerSpec defines ownership information
type OwnerSpec struct {
	Team  string `json:"team"`
	Email string `json:"email"`
	Slack string `json:"slack,omitempty"`
}

// GitHubSpec defines GitHub integration settings
type GitHubSpec struct {
	// AutoDeploy enables automatic deployment on new releases
	AutoDeploy bool `json:"autoDeploy,omitempty"`

	// WebhookSecret for GitHub webhooks
	WebhookSecret string `json:"webhookSecret,omitempty"`

	// BranchProtection settings
	BranchProtection bool `json:"branchProtection,omitempty"`
}

// ApplicationClaimStatus defines the observed state of ApplicationClaim
type ApplicationClaimStatus struct {
	// Phase of the deployment (Pending, Provisioning, Ready, Failed)
	Phase string `json:"phase,omitempty"`

	// Applications deployment status
	Applications []ApplicationStatus `json:"applications,omitempty"`

	// PlatformComponents provisioning status
	PlatformComponents []ComponentStatus `json:"platformComponents,omitempty"`

	// Endpoints for accessing applications
	Endpoints []EndpointStatus `json:"endpoints,omitempty"`

	// Conditions for detailed status
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdated timestamp
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// ObservedGeneration for tracking updates
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ApplicationStatus tracks individual application status
type ApplicationStatus struct {
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	Status          string      `json:"status"`
	Ready           bool        `json:"ready"`
	AvailableReplicas int32     `json:"availableReplicas,omitempty"`
	LastDeployed    metav1.Time `json:"lastDeployed,omitempty"`
	Message         string      `json:"message,omitempty"`
}

// ComponentStatus tracks platform component status
type ComponentStatus struct {
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Status        string                 `json:"status"`
	Ready         bool                   `json:"ready"`
	ConnectionInfo map[string]string     `json:"connectionInfo,omitempty"`
	Message       string                 `json:"message,omitempty"`
}

// EndpointStatus provides access information
type EndpointStatus struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Internal string `json:"internal,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={platform,apps}
// +kubebuilder:printcolumn:name="Environment",type="string",JSONPath=".spec.environment"
// +kubebuilder:printcolumn:name="Applications",type="string",JSONPath=".status.phase",priority=1
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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

func init() {
	SchemeBuilder.Register(&ApplicationClaim{}, &ApplicationClaimList{})
}