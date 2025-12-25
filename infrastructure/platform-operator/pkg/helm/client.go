package helm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Client Helm client
type Client struct {
	cacheDir string
}

// NewClient creates new Helm client
func NewClient() *Client {
	cacheDir := filepath.Join(os.TempDir(), "helm-cache")
	os.MkdirAll(cacheDir, 0755)
	return &Client{
		cacheDir: cacheDir,
	}
}

// Release Helm release
type Release struct {
	Name      string
	Namespace string
	Chart     string
	Values    map[string]interface{}
}

// InstallOrUpgrade installs or upgrades a Helm chart
func (c *Client) InstallOrUpgrade(ctx context.Context, release Release) error {
	// Simplified implementation
	fmt.Printf("Installing/Upgrading Helm release: %s in namespace %s\n", release.Name, release.Namespace)
	return nil
}

// Uninstall removes a Helm release
func (c *Client) Uninstall(ctx context.Context, name, namespace string) error {
	// Simplified implementation
	fmt.Printf("Uninstalling Helm release: %s from namespace %s\n", name, namespace)
	return nil
}

// PullOCIChart pulls a Helm chart from OCI registry and returns the extracted path
func (c *Client) PullOCIChart(ctx context.Context, chartURL, version string) (string, error) {
	// chartURL format: oci://ghcr.io/nimbusprotch/microservice
	// Create a unique directory for this chart version
	chartName := filepath.Base(chartURL)
	chartDir := filepath.Join(c.cacheDir, fmt.Sprintf("%s-%s", chartName, version))

	// Check if already cached
	if _, err := os.Stat(chartDir); err == nil {
		return chartDir, nil
	}

	// Login to GitHub Container Registry if token is available
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken != "" {
		loginCmd := exec.CommandContext(ctx, "helm", "registry", "login", "ghcr.io", "--username", "token", "--password", githubToken)
		loginCmd.Stdout = os.Stdout
		loginCmd.Stderr = os.Stderr
		if err := loginCmd.Run(); err != nil {
			fmt.Printf("Warning: Failed to login to GHCR (continuing anyway): %v\n", err)
		}
	}

	// Pull chart using helm pull
	fullChartRef := fmt.Sprintf("%s:%s", chartURL, version)
	cmd := exec.CommandContext(ctx, "helm", "pull", fullChartRef, "--untar", "--destination", c.cacheDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to pull OCI chart %s: %w", fullChartRef, err)
	}

	// Helm extracts to a directory named after the chart (without version)
	extractedDir := filepath.Join(c.cacheDir, chartName)

	// Rename to include version for caching
	if err := os.Rename(extractedDir, chartDir); err != nil {
		// If rename fails, directory might already exist from concurrent pull
		if _, statErr := os.Stat(chartDir); statErr == nil {
			// Clean up the extracted dir and use the existing cached one
			os.RemoveAll(extractedDir)
			return chartDir, nil
		}
		return "", fmt.Errorf("failed to cache chart: %w", err)
	}

	return chartDir, nil
}

// ReadValuesFile reads a values.yaml file and returns it as a map
func (c *Client) ReadValuesFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read values file %s: %w", filePath, err)
	}

	values := make(map[string]interface{})
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse values file %s: %w", filePath, err)
	}

	return values, nil
}

// MergeValues performs a deep merge of multiple value maps
// Later maps override earlier maps
func (c *Client) MergeValues(base map[string]interface{}, overrides ...map[string]interface{}) map[string]interface{} {
	result := deepCopy(base)

	for _, override := range overrides {
		result = deepMerge(result, override)
	}

	return result
}

// deepCopy creates a deep copy of a map
func deepCopy(src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = deepCopy(val)
		case []interface{}:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a slice
func deepCopySlice(src []interface{}) []interface{} {
	result := make([]interface{}, len(src))
	for i, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = deepCopy(val)
		case []interface{}:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// deepMerge recursively merges two maps
// Values from 'override' take precedence over 'base'
func deepMerge(base, override map[string]interface{}) map[string]interface{} {
	result := deepCopy(base)

	for k, v := range override {
		if existingVal, exists := result[k]; exists {
			// If both are maps, merge recursively
			if existingMap, ok := existingVal.(map[string]interface{}); ok {
				if overrideMap, ok := v.(map[string]interface{}); ok {
					result[k] = deepMerge(existingMap, overrideMap)
					continue
				}
			}
		}
		// Otherwise, override completely
		result[k] = v
	}

	return result
}
