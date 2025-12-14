package promise

// Promise represents a Kratix Promise resource
type Promise struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   Metadata          `yaml:"metadata"`
	Spec       PromiseSpec       `yaml:"spec"`
}

type Metadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

type PromiseSpec struct {
	API        API        `yaml:"api"`
	Workflows  Workflows  `yaml:"workflows"`
}

type API struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name       string `yaml:"name"`
		Plural     string `yaml:"plural"`
		Singular   string `yaml:"singular,omitempty"`
		ShortNames []string `yaml:"shortNames,omitempty"`
	} `yaml:"metadata"`
	Schema struct {
		OpenAPIV3Schema Schema `yaml:"openAPIV3Schema"`
	} `yaml:"schema"`
}

type Schema struct {
	Type       string               `yaml:"type"`
	Properties map[string]Property  `yaml:"properties"`
	Required   []string            `yaml:"required,omitempty"`
}

type Property struct {
	Type        string              `yaml:"type"`
	Description string              `yaml:"description,omitempty"`
	Default     interface{}         `yaml:"default,omitempty"`
	Properties  map[string]Property `yaml:"properties,omitempty"`
	Items       *Property           `yaml:"items,omitempty"`
	Required    []string            `yaml:"required,omitempty"`
}

type Workflows struct {
	Resource ResourceWorkflow `yaml:"resource"`
}

type ResourceWorkflow struct {
	Configure []Pipeline `yaml:"configure"`
}

type Pipeline struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   Metadata     `yaml:"metadata"`
	Spec       PipelineSpec `yaml:"spec"`
}

type PipelineSpec struct {
	Containers []Container `yaml:"containers"`
}

type Container struct {
	Name    string   `yaml:"name"`
	Image   string   `yaml:"image"`
	Command []string `yaml:"command,omitempty"`
	Args    []string `yaml:"args,omitempty"`
}

// PlatformRequest represents the user's request for platform resources
type PlatformRequest struct {
	Tenant      string            `yaml:"tenant"`
	Environment string            `yaml:"environment"`
	Components  map[string]bool   `yaml:"components"`
	Settings    map[string]interface{} `yaml:"settings,omitempty"`
}