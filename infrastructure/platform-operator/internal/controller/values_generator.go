package controller

import (
	"fmt"
	"os"
	"strings"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"gopkg.in/yaml.v3"
)

// generateValuesForApp generates environment-specific Helm values for an application
func (r *ApplicationClaimReconciler) generateValuesForApp(claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) (string, error) {
	values := make(map[string]interface{})

	// Basic app configuration
	values["fullnameOverride"] = app.Name
	values["replicaCount"] = app.Replicas

	// Image configuration
	// If app.Image is provided, use it; otherwise derive from serviceName
	imageRepo := app.Image
	if imageRepo == "" && app.ServiceName != "" {
		// Derive image repository from serviceName
		// Default: GHCR with configurable organization
		imageRegistry := os.Getenv("IMAGE_REGISTRY")
		if imageRegistry == "" {
			imageRegistry = "ghcr.io/nimbusprotch" // Default GHCR org
		}
		imageRepo = fmt.Sprintf("%s/%s", imageRegistry, app.ServiceName)
	}

	// Remove any existing tag from image
	if strings.Contains(imageRepo, ":") {
		imageRepo = strings.Split(imageRepo, ":")[0]
	}

	values["image"] = map[string]interface{}{
		"repository": imageRepo,
		"tag":        app.Version,
		"pullPolicy": "IfNotPresent",
	}

	// Image pull secrets for private images
	values["imagePullSecrets"] = []map[string]string{
		{"name": "ghcr-secret"},
	}

	// Service configuration
	if len(app.Ports) > 0 {
		values["service"] = map[string]interface{}{
			"type":       "ClusterIP",
			"port":       app.Ports[0].Port,
			"targetPort": app.Ports[0].Port,
		}
	}

	// Environment variables
	if len(app.Env) > 0 {
		envVars := []map[string]string{}
		for _, e := range app.Env {
			envVars = append(envVars, map[string]string{
				"name":  e.Name,
				"value": e.Value,
			})
		}
		values["env"] = envVars
	}

	// Environment-specific resource configuration
	values["resources"] = r.getResourcesForEnvironment(claim.Spec.Environment, "app")

	// Auto-scaling for production (disabled for now - no HPA template in chart)
	// TODO: Add HPA template to enable autoscaling
	// if claim.Spec.Environment == "prod" {
	// 	values["autoscaling"] = map[string]interface{}{
	// 		"enabled":                        true,
	// 		"minReplicas":                    app.Replicas,
	// 		"maxReplicas":                    app.Replicas * 3,
	// 		"targetCPUUtilizationPercentage": 70,
	// 	}
	// }

	// Labels
	values["labels"] = map[string]string{
		"platform.infraforge.io/managed": "true",
		"platform.infraforge.io/claim":   claim.Name,
		"platform.infraforge.io/team":    claim.Spec.Owner.Team,
		"platform.infraforge.io/env":     claim.Spec.Environment,
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to marshal values to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// generateValuesForComponent generates environment-specific Helm values for a component
func (r *ApplicationClaimReconciler) generateValuesForComponent(claim *platformv1.ApplicationClaim, component platformv1.ComponentSpec) (string, error) {
	values := make(map[string]interface{})

	// Basic configuration
	values["fullnameOverride"] = component.Name

	// Component image based on type
	switch component.Type {
	case "postgresql":
		values["image"] = map[string]interface{}{
			"repository": "postgres",
			"tag":        component.Version,
			"pullPolicy": "IfNotPresent",
		}
		// Add required postgres environment variables
		values["env"] = []map[string]string{
			{"name": "POSTGRES_PASSWORD", "value": "postgres"},
			{"name": "POSTGRES_USER", "value": "postgres"},
			{"name": "POSTGRES_DB", "value": "postgres"},
		}
		// Add TCP probes for postgres
		values["livenessProbe"] = map[string]interface{}{
			"tcpSocket": map[string]interface{}{
				"port": 5432,
			},
			"initialDelaySeconds": 30,
			"periodSeconds":       10,
		}
		values["readinessProbe"] = map[string]interface{}{
			"tcpSocket": map[string]interface{}{
				"port": 5432,
			},
			"initialDelaySeconds": 5,
			"periodSeconds":       5,
		}
	case "redis":
		values["image"] = map[string]interface{}{
			"repository": "redis",
			"tag":        component.Version,
			"pullPolicy": "IfNotPresent",
		}
		// Add TCP probes for redis
		values["livenessProbe"] = map[string]interface{}{
			"tcpSocket": map[string]interface{}{
				"port": 6379,
			},
			"initialDelaySeconds": 30,
			"periodSeconds":       10,
		}
		values["readinessProbe"] = map[string]interface{}{
			"tcpSocket": map[string]interface{}{
				"port": 6379,
			},
			"initialDelaySeconds": 5,
			"periodSeconds":       5,
		}
	case "rabbitmq":
		values["image"] = map[string]interface{}{
			"repository": "rabbitmq",
			"tag":        component.Version,
			"pullPolicy": "IfNotPresent",
		}
		// Add default rabbitmq credentials
		values["env"] = []map[string]string{
			{"name": "RABBITMQ_DEFAULT_USER", "value": "guest"},
			{"name": "RABBITMQ_DEFAULT_PASS", "value": "guest"},
		}
		// Add TCP probes for rabbitmq
		values["livenessProbe"] = map[string]interface{}{
			"tcpSocket": map[string]interface{}{
				"port": 5672,
			},
			"initialDelaySeconds": 30,
			"periodSeconds":       10,
		}
		values["readinessProbe"] = map[string]interface{}{
			"tcpSocket": map[string]interface{}{
				"port": 5672,
			},
			"initialDelaySeconds": 5,
			"periodSeconds":       5,
		}
	}

	// Image pull secrets for private images (not needed for public Docker Hub images but keeping for consistency)
	values["imagePullSecrets"] = []map[string]string{
		{"name": "ghcr-secret"},
	}

	// Environment-specific configuration
	switch claim.Spec.Environment {
	case "prod":
		values["replicaCount"] = 3
		values["resources"] = r.getResourcesForEnvironment("prod", component.Type)

		// Production-specific settings
		switch component.Type {
		case "postgresql":
			values["persistence"] = map[string]interface{}{
				"enabled": true,
				"size":    "100Gi",
			}
			values["backup"] = map[string]interface{}{
				"enabled": true,
			}
		case "redis":
			values["master"] = map[string]interface{}{
				"persistence": map[string]interface{}{
					"enabled": true,
					"size":    "10Gi",
				},
			}
			values["replica"] = map[string]interface{}{
				"replicaCount": 2,
				"persistence": map[string]interface{}{
					"enabled": true,
					"size":    "10Gi",
				},
			}
		case "rabbitmq":
			values["replicaCount"] = 3
			values["persistence"] = map[string]interface{}{
				"enabled": true,
				"size":    "20Gi",
			}
		}

	case "staging":
		values["replicaCount"] = 2
		values["resources"] = r.getResourcesForEnvironment("staging", component.Type)

		switch component.Type {
		case "postgresql", "redis", "rabbitmq":
			values["persistence"] = map[string]interface{}{
				"enabled": true,
				"size":    "20Gi",
			}
		}

	default: // dev
		values["replicaCount"] = 1
		values["resources"] = r.getResourcesForEnvironment("dev", component.Type)

		switch component.Type {
		case "postgresql", "redis", "rabbitmq":
			values["persistence"] = map[string]interface{}{
				"enabled": true,
				"size":    "10Gi",
			}
		}
	}

	// Apply custom config from claim
	// Note: This is old code - custom config already handled above via JSON unmarshal

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to marshal values to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// getResourcesForEnvironment returns resource limits/requests based on environment and type
func (r *ApplicationClaimReconciler) getResourcesForEnvironment(environment, resourceType string) map[string]interface{} {
	resources := make(map[string]interface{})

	switch environment {
	case "prod":
		resources["requests"] = map[string]string{
			"memory": "1Gi",
			"cpu":    "500m",
		}
		resources["limits"] = map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		}

	case "staging":
		resources["requests"] = map[string]string{
			"memory": "512Mi",
			"cpu":    "250m",
		}
		resources["limits"] = map[string]string{
			"memory": "1Gi",
			"cpu":    "500m",
		}

	default: // dev
		resources["requests"] = map[string]string{
			"memory": "256Mi",
			"cpu":    "100m",
		}
		resources["limits"] = map[string]string{
			"memory": "512Mi",
			"cpu":    "250m",
		}
	}

	return resources
}
