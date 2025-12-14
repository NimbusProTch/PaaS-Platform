package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateGenericApp generates manifests for any application type
func (p *InfraForgeProcessor) generateGenericApp(dir, appName, profile string) error {
	if profile == "" {
		profile = "dev"
	}
	
	// Check if we have templates for this app
	templateDir := fmt.Sprintf("/platform-templates/%s", appName)
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		// No templates, generate a simple manifest
		return p.generateSimpleApp(dir, appName, profile)
	}
	
	// Load profile values
	profileData, err := p.loadProfileValues(appName, profile)
	if err != nil {
		// Use defaults if no profile exists
		profileData = p.getDefaultValues(profile)
	}
	
	// Prepare template data
	templateData := map[string]interface{}{
		"Name":        p.request.Metadata.Name,
		"AppName":     appName,
		"Namespace":   fmt.Sprintf("%s-%s", p.request.Spec.Tenant, p.request.Spec.Environment),
		"Tenant":      p.request.Spec.Tenant,
		"Environment": p.request.Spec.Environment,
		"Profile":     profile,
		"Values":      profileData,
	}
	
	// List all templates in the app directory
	templates, err := os.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}
	
	// Create kustomization.yaml
	kustomizationFile := filepath.Join(dir, "kustomization.yaml")
	kustomization := map[string]interface{}{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  []string{},
	}
	
	resources := []string{}
	
	// Generate each template
	for _, tmpl := range templates {
		if !strings.HasSuffix(tmpl.Name(), ".tmpl") || tmpl.IsDir() {
			continue
		}
		
		// Skip profile files
		if strings.Contains(tmpl.Name(), "profiles/") {
			continue
		}
		
		outputName := strings.TrimSuffix(tmpl.Name(), ".tmpl") + ".yaml"
		outputFile := filepath.Join(dir, outputName)
		
		if err := p.generateFromTemplate(appName, tmpl.Name(), outputFile, templateData); err != nil {
			// Log error but continue with other templates
			fmt.Printf("Warning: failed to generate %s: %v\n", tmpl.Name(), err)
			continue
		}
		
		resources = append(resources, outputName)
	}
	
	// Update kustomization with resources
	kustomization["resources"] = resources
	
	return p.writeYAML(kustomizationFile, kustomization)
}

// generateSimpleApp generates a basic deployment for apps without templates
func (p *InfraForgeProcessor) generateSimpleApp(dir, appName, profile string) error {
	values := p.getDefaultValues(profile)
	namespace := fmt.Sprintf("%s-%s", p.request.Spec.Tenant, p.request.Spec.Environment)
	
	// Create deployment
	deploymentFile := filepath.Join(dir, "deployment.yaml")
	deployment := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      appName,
			"namespace": namespace,
			"labels": map[string]interface{}{
				"app":         appName,
				"tenant":      p.request.Spec.Tenant,
				"environment": p.request.Spec.Environment,
			},
		},
		"spec": map[string]interface{}{
			"replicas": values["replicaCount"],
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": appName,
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app":         appName,
						"tenant":      p.request.Spec.Tenant,
						"environment": p.request.Spec.Environment,
					},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  appName,
							"image": fmt.Sprintf("nginx:1.25"),
							"ports": []map[string]interface{}{
								{"containerPort": 80},
							},
							"resources": values["resources"],
						},
					},
				},
			},
		},
	}
	
	// Create service
	serviceFile := filepath.Join(dir, "service.yaml")
	service := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      appName,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": appName,
			},
			"ports": []map[string]interface{}{
				{
					"port":       80,
					"targetPort": 80,
				},
			},
		},
	}
	
	// Create kustomization
	kustomizationFile := filepath.Join(dir, "kustomization.yaml")
	kustomization := map[string]interface{}{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources": []string{
			"deployment.yaml",
			"service.yaml",
		},
	}
	
	if err := p.writeYAML(deploymentFile, deployment); err != nil {
		return err
	}
	
	if err := p.writeYAML(serviceFile, service); err != nil {
		return err
	}
	
	return p.writeYAML(kustomizationFile, kustomization)
}

// getDefaultValues returns default values based on profile
func (p *InfraForgeProcessor) getDefaultValues(profile string) map[string]interface{} {
	switch profile {
	case "production":
		return map[string]interface{}{
			"replicaCount": 3,
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "500m",
					"memory": "512Mi",
				},
				"requests": map[string]interface{}{
					"cpu":    "250m",
					"memory": "256Mi",
				},
			},
		}
	case "standard":
		return map[string]interface{}{
			"replicaCount": 2,
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "200m",
					"memory": "256Mi",
				},
				"requests": map[string]interface{}{
					"cpu":    "100m",
					"memory": "128Mi",
				},
			},
		}
	default: // dev
		return map[string]interface{}{
			"replicaCount": 1,
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "100m",
					"memory": "128Mi",
				},
				"requests": map[string]interface{}{
					"cpu":    "50m",
					"memory": "64Mi",
				},
			},
		}
	}
}