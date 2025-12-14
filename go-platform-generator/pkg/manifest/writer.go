package manifest

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WriteYAML writes an object to a YAML file
func WriteYAML(path string, obj interface{}) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal to YAML
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}

	// Write file
	return os.WriteFile(path, data, 0644)
}

// ToYAMLString converts an object to a YAML string
func ToYAMLString(obj interface{}) string {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(data)
}