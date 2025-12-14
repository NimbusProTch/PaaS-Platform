package promise

import (
	"fmt"
)

type Generator struct {
	components map[string]Component
}

type Component struct {
	Name        string                 `yaml:"name"`
	DisplayName string                 `yaml:"displayName"`
	Description string                 `yaml:"description"`
	Category    string                 `yaml:"category"`
	Type        string                 `yaml:"type"`
	Helm        HelmConfig             `yaml:"helm"`
	Parameters  []Parameter            `yaml:"parameters"`
}

type HelmConfig struct {
	Repository string `yaml:"repository"`
	Chart      string `yaml:"chart"`
	Version    string `yaml:"version"`
}

type Parameter struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
}

func NewGenerator() *Generator {
	return &Generator{
		components: make(map[string]Component),
	}
}

func (g *Generator) GeneratePlatformPromise() (*Promise, error) {
	promise := &Promise{
		APIVersion: "platform.kratix.io/v1",
		Kind:       "Promise",
		Metadata: Metadata{
			Name: "platform-stack",
			Labels: map[string]string{
				"type": "platform",
			},
		},
		Spec: PromiseSpec{
			API: g.generateAPI(),
			Workflows: Workflows{
				Resource: ResourceWorkflow{
					Configure: []Pipeline{g.generatePipeline()},
				},
			},
		},
	}
	
	return promise, nil
}

func (g *Generator) generateAPI() API {
	return API{
		APIVersion: "apiextensions.k8s.io/v1",
		Kind:       "CustomResourceDefinition",
		Metadata: struct {
			Name       string   `yaml:"name"`
			Plural     string   `yaml:"plural"`
			Singular   string   `yaml:"singular,omitempty"`
			ShortNames []string `yaml:"shortNames,omitempty"`
		}{
			Name:       "platformstacks.platform.example.com",
			Plural:     "platformstacks",
			Singular:   "platformstack",
			ShortNames: []string{"ps", "stack"},
		},
		Schema: struct {
			OpenAPIV3Schema Schema `yaml:"openAPIV3Schema"`
		}{
			OpenAPIV3Schema: g.generateSchema(),
		},
	}
}

func (g *Generator) generateSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"spec": {
				Type:        "object",
				Description: "Platform stack specification",
				Properties: map[string]Property{
					"tenant": {
						Type:        "string",
						Description: "Tenant name",
					},
					"environment": {
						Type:        "string",
						Description: "Environment (dev, staging, prod)",
						Default:     "dev",
					},
					"components": {
						Type:        "object",
						Description: "Components to enable/disable",
						Properties:  g.generateComponentProperties(),
					},
				},
				Required: []string{"tenant", "components"},
			},
		},
	}
}

func (g *Generator) generateComponentProperties() map[string]Property {
	props := make(map[string]Property)
	
	// Add all platform components
	components := []string{"redis", "nginx", "postgresql", "keycloak"}
	
	for _, comp := range components {
		props[comp] = Property{
			Type:        "object",
			Description: fmt.Sprintf("Configuration for %s", comp),
			Properties: map[string]Property{
				"enabled": {
					Type:        "boolean",
					Description: fmt.Sprintf("Enable %s", comp),
					Default:     false,
				},
				"version": {
					Type:        "string",
					Description: "Helm chart version",
				},
				"values": {
					Type:        "object",
					Description: "Helm values override",
				},
			},
		}
	}
	
	return props
}

func (g *Generator) generatePipeline() Pipeline {
	return Pipeline{
		APIVersion: "platform.kratix.io/v1",
		Kind:       "Pipeline",
		Metadata: Metadata{
			Name: "platform-configure",
		},
		Spec: PipelineSpec{
			Containers: []Container{
				{
					Name:  "generate-manifests",
					Image: "ghcr.io/gaskin/platform-generator:latest",
					Command: []string{"/app/generator"},
					Args: []string{
						"pipeline",
						"--input", "/kratix/object.yaml",
						"--output", "/kratix/output",
					},
				},
			},
		},
	}
}