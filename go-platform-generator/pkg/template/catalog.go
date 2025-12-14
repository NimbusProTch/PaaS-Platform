package template

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Catalog represents a service catalog definition
type Catalog struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   CatalogMeta    `yaml:"metadata"`
	Spec       CatalogSpec    `yaml:"spec"`
}

type CatalogMeta struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"displayName"`
	Description string `yaml:"description"`
}

type CatalogSpec struct {
	Type            string                 `yaml:"type"`
	Category        string                 `yaml:"category"`
	AzureEquivalent *AzureEquivalent       `yaml:"azureEquivalent,omitempty"`
	Dependencies    []Dependency           `yaml:"dependencies,omitempty"`
	Profiles        []Profile              `yaml:"profiles"`
	Parameters      map[string]interface{} `yaml:"parameters,omitempty"`
	Installation    *Installation          `yaml:"installation,omitempty"`
}

type AzureEquivalent struct {
	Service     string `yaml:"service"`
	Description string `yaml:"description"`
}

type Dependency struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	Version string `yaml:"version"`
}

type Profile struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Features    []string `yaml:"features"`
	Default     bool     `yaml:"default,omitempty"`
}

type Installation struct {
	Method     string `yaml:"method"`
	Repository string `yaml:"repository"`
	Chart      string `yaml:"chart"`
	Version    string `yaml:"version"`
	Namespace  string `yaml:"namespace"`
}

// CatalogLoader loads service catalogs from disk
type CatalogLoader struct {
	templatesRoot string
}

// NewCatalogLoader creates a new catalog loader
func NewCatalogLoader(templatesRoot string) *CatalogLoader {
	return &CatalogLoader{
		templatesRoot: templatesRoot,
	}
}

// LoadOperatorCatalog loads an operator catalog
func (cl *CatalogLoader) LoadOperatorCatalog(operatorName string) (*Catalog, error) {
	catalogPath := filepath.Join(cl.templatesRoot, "operators", operatorName, "catalog.yaml")
	return cl.loadCatalog(catalogPath)
}

// LoadServiceCatalog loads a service catalog
func (cl *CatalogLoader) LoadServiceCatalog(serviceName string) (*Catalog, error) {
	catalogPath := filepath.Join(cl.templatesRoot, "services", serviceName, "catalog.yaml")
	return cl.loadCatalog(catalogPath)
}

// loadCatalog reads and parses a catalog file
func (cl *CatalogLoader) loadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog %s: %w", path, err)
	}

	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog %s: %w", path, err)
	}

	return &catalog, nil
}

// GetDefaultProfile returns the default profile for a catalog
func (c *Catalog) GetDefaultProfile() string {
	for _, profile := range c.Spec.Profiles {
		if profile.Default {
			return profile.Name
		}
	}

	// If no default specified, return first profile
	if len(c.Spec.Profiles) > 0 {
		return c.Spec.Profiles[0].Name
	}

	return "nonprod"
}
