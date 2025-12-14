package template

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TemplateRenderer renders Helm templates and Kubernetes manifests
type TemplateRenderer struct {
	templatesRoot string
	catalogLoader *CatalogLoader
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer(templatesRoot string) *TemplateRenderer {
	return &TemplateRenderer{
		templatesRoot: templatesRoot,
		catalogLoader: NewCatalogLoader(templatesRoot),
	}
}

// RenderOperator renders an operator installation
func (tr *TemplateRenderer) RenderOperator(operatorName string, values map[string]interface{}) ([]byte, error) {
	catalog, err := tr.catalogLoader.LoadOperatorCatalog(operatorName)
	if err != nil {
		return nil, fmt.Errorf("failed to load operator catalog: %w", err)
	}

	operatorDir := filepath.Join(tr.templatesRoot, "operators", operatorName)

	// For operators, we just need to copy the Helm chart reference
	// The actual installation is done by Helm
	chartPath := filepath.Join(operatorDir, "Chart.yaml")
	valuesPath := filepath.Join(operatorDir, "values.yaml")

	// Read Chart.yaml
	chartData, err := os.ReadFile(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read operator Chart.yaml: %w", err)
	}

	// Read values.yaml
	var operatorValues map[string]interface{}
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read operator values.yaml: %w", err)
	}
	if err := yaml.Unmarshal(valuesData, &operatorValues); err != nil {
		return nil, fmt.Errorf("failed to parse operator values: %w", err)
	}

	// Merge with provided values
	mergedValues := mergeMaps(operatorValues, values)

	// Create output structure
	output := map[string]interface{}{
		"chart":   string(chartData),
		"values":  mergedValues,
		"catalog": catalog,
	}

	return yaml.Marshal(output)
}

// RenderService copies Helm chart structure with merged values
// Returns the chart directory for ArgoCD to deploy as Helm release
func (tr *TemplateRenderer) RenderService(serviceName, profile string, values map[string]interface{}) (string, map[string]interface{}, error) {
	catalog, err := tr.catalogLoader.LoadServiceCatalog(serviceName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load service catalog: %w", err)
	}

	// Use default profile if not specified
	if profile == "" {
		profile = catalog.GetDefaultProfile()
	}

	serviceDir := filepath.Join(tr.templatesRoot, "services", serviceName, profile)

	// Check if profile directory exists
	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return "", nil, fmt.Errorf("profile %s not found for service %s", profile, serviceName)
	}

	// Read values.yaml
	valuesPath := filepath.Join(serviceDir, "values.yaml")
	var profileValues map[string]interface{}
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read values.yaml: %w", err)
	}
	if err := yaml.Unmarshal(valuesData, &profileValues); err != nil {
		return "", nil, fmt.Errorf("failed to parse values.yaml: %w", err)
	}

	// Merge with provided values
	mergedValues := mergeMaps(profileValues, values)

	// Return source directory path and merged values
	// ArgoCD will deploy this as a Helm chart, NOT raw manifests
	return serviceDir, mergedValues, nil
}

// renderHelmTemplates renders Helm templates in a directory
func (tr *TemplateRenderer) renderHelmTemplates(templatesDir string, chart map[string]interface{}, values map[string]interface{}) ([][]byte, error) {
	// Create a temporary directory for rendering
	tmpDir, err := os.MkdirTemp("", "helm-render-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Chart.yaml
	chartPath := filepath.Join(tmpDir, "Chart.yaml")
	chartData, _ := yaml.Marshal(chart)
	if err := os.WriteFile(chartPath, chartData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write Chart.yaml: %w", err)
	}

	// Write values.yaml
	valuesPath := filepath.Join(tmpDir, "values.yaml")
	valuesData, _ := yaml.Marshal(values)
	if err := os.WriteFile(valuesPath, valuesData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write values.yaml: %w", err)
	}

	// Copy templates directory
	tmpTemplatesDir := filepath.Join(tmpDir, "templates")
	if err := copyDir(templatesDir, tmpTemplatesDir); err != nil {
		return nil, fmt.Errorf("failed to copy templates: %w", err)
	}

	// Use helm template command
	cmd := exec.Command("helm", "template", "release", tmpDir, "-f", valuesPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template failed: %v\nstderr: %s", err, stderr.String())
	}

	// Split output into individual manifests
	manifests := splitYAMLDocuments(stdout.Bytes())
	return manifests, nil
}

// mergeMaps merges two maps, with override taking precedence
func mergeMaps(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Override with new values
	for k, v := range override {
		if baseMap, ok := result[k].(map[string]interface{}); ok {
			if overrideMap, ok := v.(map[string]interface{}); ok {
				result[k] = mergeMaps(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, 0644)
	})
}

// splitYAMLDocuments splits a multi-document YAML file
func splitYAMLDocuments(data []byte) [][]byte {
	var documents [][]byte
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var doc interface{}
		if err := decoder.Decode(&doc); err != nil {
			break
		}

		if doc != nil {
			docData, _ := yaml.Marshal(doc)
			documents = append(documents, docData)
		}
	}

	return documents
}
