package controller

import (
	"regexp"
	"strings"
)

// normalizeK8sName converts a name to K8s-compliant format (lowercase, alphanumeric + dash)
func normalizeK8sName(name string) string {
	// Convert to lowercase
	normalized := strings.ToLower(name)

	// Replace spaces and underscores with dashes
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")

	// Remove any characters that aren't alphanumeric or dash
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	normalized = reg.ReplaceAllString(normalized, "")

	// Remove leading/trailing dashes
	normalized = strings.Trim(normalized, "-")

	// Ensure it doesn't end with a dash
	for strings.HasSuffix(normalized, "--") {
		normalized = strings.TrimSuffix(normalized, "-")
	}

	return normalized
}
