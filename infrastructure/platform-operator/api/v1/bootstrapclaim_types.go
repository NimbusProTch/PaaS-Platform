// +kubebuilder:object:generate=true
// +groupName=platform.infraforge.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BootstrapClaimSpec defines the desired state of BootstrapClaim
type BootstrapClaimSpec struct {
	// GiteaURL is the URL of the Gitea server
	GiteaURL string `json:"giteaURL"`

	// Organization is the Gitea organization name
	Organization string `json:"organization"`

	// Repositories to create and initialize
	Repositories RepositoriesSpec `json:"repositories"`

	// GitOps configuration
	GitOps GitOpsSpec `json:"gitOps"`
}

// RepositoriesSpec defines the repositories to create
type RepositoriesSpec struct {
	// Charts repository name - contains both microservice and platform templates (default: "charts")
	Charts string `json:"charts,omitempty"`

	// Voltran GitOps config repository name (default: "voltran")
	Voltran string `json:"voltran,omitempty"`
}

// GitOpsSpec defines GitOps configuration
type GitOpsSpec struct {
	// Branch name for GitOps (default: "main")
	Branch string `json:"branch,omitempty"`

	// Environments to create (default: ["dev", "qa", "sandbox", "staging", "prod"])
	Environments []string `json:"environments,omitempty"`

	// ClusterType for root app generation (nonprod/prod)
	ClusterType string `json:"clusterType,omitempty"`
}

// BootstrapClaimStatus defines the observed state of BootstrapClaim
type BootstrapClaimStatus struct {
	// Phase current phase (Pending, Bootstrapping, Ready, Failed)
	Phase string `json:"phase,omitempty"`

	// Ready overall readiness status
	Ready bool `json:"ready"`

	// RepositoriesCreated tracks repository creation
	RepositoriesCreated bool `json:"repositoriesCreated"`

	// ChartsUploaded tracks chart upload status
	ChartsUploaded bool `json:"chartsUploaded"`

	// RootAppGenerated tracks root app generation
	RootAppGenerated bool `json:"rootAppGenerated"`

	// Message provides additional status information
	Message string `json:"message,omitempty"`

	// Conditions detailed conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdated last update timestamp
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// RepositoryURLs created repository URLs
	RepositoryURLs map[string]string `json:"repositoryURLs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BootstrapClaim is the Schema for the bootstrapclaims API
type BootstrapClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BootstrapClaimSpec   `json:"spec,omitempty"`
	Status BootstrapClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BootstrapClaimList contains a list of BootstrapClaim
type BootstrapClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootstrapClaim `json:"items"`
}
