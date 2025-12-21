package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
)

var (
	argoCDApplicationGVK = schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	}
)

// ArgoCD controller - Platform Operator sadece ArgoCD Application oluşturur
// Tüm deployment'lar ArgoCD tarafından yapılır
func (r *ApplicationClaimReconciler) reconcileWithArgoCD(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ApplicationClaim with ArgoCD", "name", claim.Name)

	// 1. Create namespace for the team
	if err := r.ensureNamespace(ctx, claim.Spec.Namespace); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// 2. Create ArgoCD Project for team isolation
	if err := r.createArgoCDProject(ctx, claim); err != nil {
		return fmt.Errorf("failed to create ArgoCD project: %w", err)
	}

	// 2.5. Ensure required operators are installed
	if err := r.ensureOperatorsInstalled(ctx, claim); err != nil {
		return fmt.Errorf("failed to ensure operators are installed: %w", err)
	}

	// 3. Generate and store Helm values for each application
	for _, app := range claim.Spec.Applications {
		valuesYAML, err := r.generateValuesForApp(claim, app)
		if err != nil {
			logger.Error(err, "Failed to generate values for app", "app", app.Name)
			return fmt.Errorf("failed to generate values for app %s: %w", app.Name, err)
		}

		if err := r.storeValuesInConfigMap(ctx, claim, app.Name, valuesYAML); err != nil {
			logger.Error(err, "Failed to store values for app", "app", app.Name)
			return fmt.Errorf("failed to store values for app %s: %w", app.Name, err)
		}
		logger.Info("Generated and stored values for app", "app", app.Name)
	}

	// 4. Generate and store Helm values for each component
	for _, component := range claim.Spec.Components {
		valuesYAML, err := r.generateValuesForComponent(claim, component)
		if err != nil {
			logger.Error(err, "Failed to generate values for component", "component", component.Name)
			return fmt.Errorf("failed to generate values for component %s: %w", component.Name, err)
		}

		if err := r.storeValuesInConfigMap(ctx, claim, component.Name, valuesYAML); err != nil {
			logger.Error(err, "Failed to store values for component", "component", component.Name)
			return fmt.Errorf("failed to store values for component %s: %w", component.Name, err)
		}
		logger.Info("Generated and stored values for component", "component", component.Name)
	}

	// 5. Create ApplicationSet for this claim (one per environment/claim)
	if err := r.createApplicationSet(ctx, claim); err != nil {
		logger.Error(err, "Failed to create ApplicationSet")
		return fmt.Errorf("failed to create ApplicationSet: %w", err)
	}
	logger.Info("Created ArgoCD ApplicationSet", "claim", claim.Name, "environment", claim.Spec.Environment)

	return nil
}

// Create ArgoCD Project for team isolation
func (r *ApplicationClaimReconciler) createArgoCDProject(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)

	// Normalize team name for K8s naming (lowercase, no spaces)
	teamName := normalizeK8sName(claim.Spec.Owner.Team)
	projectName := fmt.Sprintf("%s-%s", teamName, claim.Spec.Environment)

	logger.Info("Creating ArgoCD project", "name", projectName)

	project := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "AppProject",
			"metadata": map[string]interface{}{
				"name":      projectName,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed": "true",
					"platform.infraforge.io/team":    teamName,
					"platform.infraforge.io/env":     claim.Spec.Environment,
				},
			},
			"spec": map[string]interface{}{
				"description": fmt.Sprintf("Project for %s team in %s environment", claim.Spec.Owner.Team, claim.Spec.Environment),
				"sourceRepos": []string{
					"*", // Allow all repos - can be restricted later
				},
				"destinations": []map[string]interface{}{
					{
						"namespace": claim.Spec.Namespace,
						"server":    "https://kubernetes.default.svc",
					},
				},
				"clusterResourceWhitelist": []map[string]interface{}{
					{
						"group": "*",
						"kind":  "*",
					},
				},
				"namespaceResourceWhitelist": []map[string]interface{}{
					{
						"group": "*",
						"kind":  "*",
					},
				},
				"roles": []map[string]interface{}{
					{
						"name": "admin",
						"policies": []string{
							fmt.Sprintf("p, proj:%s-%s:admin, applications, *, %s-%s/*, allow",
								claim.Spec.Owner.Team, claim.Spec.Environment,
								claim.Spec.Owner.Team, claim.Spec.Environment),
						},
						"groups": []string{
							claim.Spec.Owner.Team,
						},
					},
				},
			},
		},
	}

	project.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "AppProject",
	})

	// Check if project exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(project.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{
		Name:      project.GetName(),
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ArgoCD project", "name", project.GetName())
			return r.Create(ctx, project)
		}
		return err
	}

	// Update if exists
	project.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, project)
}

// Create ArgoCD Application for business applications
func (r *ApplicationClaimReconciler) createArgoCDApplication(ctx context.Context, claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) error {
	logger := log.FromContext(ctx)
	appName := fmt.Sprintf("%s-%s", claim.Spec.Namespace, app.Name)

	// Git repository URL - can be from app spec or default org repo
	repoURL := app.Repository
	if repoURL == "" {
		repoURL = os.Getenv("GIT_REPO_URL")
		if repoURL == "" {
			repoURL = "https://github.com/infraforge/platform-configs"
		}
	}

	// Path in git repo
	path := fmt.Sprintf("teams/%s/environments/%s/applications/%s",
		strings.ToLower(strings.ReplaceAll(claim.Spec.Owner.Team, " ", "-")),
		claim.Spec.Environment,
		app.Name)

	// Create ArgoCD Application
	application := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      appName,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed":     "true",
					"platform.infraforge.io/team":        claim.Spec.Owner.Team,
					"platform.infraforge.io/env":         claim.Spec.Environment,
					"platform.infraforge.io/application": app.Name,
					"platform.infraforge.io/version":     app.Version,
				},
				"finalizers": []string{
					"resources-finalizer.argocd.argoproj.io",
				},
			},
			"spec": map[string]interface{}{
				"project": fmt.Sprintf("%s-%s", claim.Spec.Owner.Team, claim.Spec.Environment),
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"path":           path,
					"targetRevision": app.Version, // Use version as git tag/branch
					"helm": map[string]interface{}{
						"valueFiles": []string{
							"values.yaml",
							fmt.Sprintf("values-%s.yaml", claim.Spec.Environment),
						},
						"parameters": r.buildHelmParameters(app, claim),
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": claim.Spec.Namespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":      true,
						"selfHeal":   true,
						"allowEmpty": false,
					},
					"syncOptions": []string{
						"CreateNamespace=true",
						"PrunePropagationPolicy=foreground",
						"PruneLast=true",
					},
					"retry": map[string]interface{}{
						"limit": 5,
						"backoff": map[string]interface{}{
							"duration":    "5s",
							"factor":      2,
							"maxDuration": "3m",
						},
					},
				},
				"revisionHistoryLimit": 10,
			},
		},
	}

	application.SetGroupVersionKind(argoCDApplicationGVK)

	// Check if application exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(argoCDApplicationGVK)
	err := r.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ArgoCD application", "name", appName)
			return r.Create(ctx, application)
		}
		return err
	}

	// Update if exists
	application.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, application)
}

// Create ArgoCD Application for infrastructure components (PostgreSQL, Redis, etc.)
func (r *ApplicationClaimReconciler) createArgoCDComponentApplication(ctx context.Context, claim *platformv1.ApplicationClaim, component platformv1.ComponentSpec) error {
	logger := log.FromContext(ctx)
	appName := fmt.Sprintf("%s-%s-%s", claim.Spec.Namespace, component.Type, component.Name)

	// Determine Helm chart for component
	var chartRepo, chartName, chartVersion string
	switch component.Type {
	case "postgresql":
		chartRepo = "https://charts.bitnami.com/bitnami"
		chartName = "postgresql"
		chartVersion = "13.2.0" // or use component.Version
	case "redis":
		chartRepo = "https://charts.bitnami.com/bitnami"
		chartName = "redis"
		chartVersion = "18.4.0"
	case "mongodb":
		chartRepo = "https://charts.bitnami.com/bitnami"
		chartName = "mongodb"
		chartVersion = "14.3.0"
	case "elasticsearch":
		chartRepo = "https://helm.elastic.co"
		chartName = "elasticsearch"
		chartVersion = "8.5.1"
	case "kafka":
		chartRepo = "https://charts.bitnami.com/bitnami"
		chartName = "kafka"
		chartVersion = "26.4.0"
	case "rabbitmq":
		chartRepo = "https://charts.bitnami.com/bitnami"
		chartName = "rabbitmq"
		chartVersion = "12.5.0"
	default:
		return fmt.Errorf("unsupported component type: %s", component.Type)
	}

	// Create ArgoCD Application for component
	application := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      appName,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed":   "true",
					"platform.infraforge.io/team":      claim.Spec.Owner.Team,
					"platform.infraforge.io/env":       claim.Spec.Environment,
					"platform.infraforge.io/component": component.Type,
					"platform.infraforge.io/instance":  component.Name,
				},
				"finalizers": []string{
					"resources-finalizer.argocd.argoproj.io",
				},
			},
			"spec": map[string]interface{}{
				"project": fmt.Sprintf("%s-%s", claim.Spec.Owner.Team, claim.Spec.Environment),
				"source": map[string]interface{}{
					"repoURL":        chartRepo,
					"chart":          chartName,
					"targetRevision": chartVersion,
					"helm": map[string]interface{}{
						"releaseName": component.Name,
						"values":      r.buildComponentValues(component, claim),
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": claim.Spec.Namespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
					"syncOptions": []string{
						"CreateNamespace=true",
						"ServerSideApply=true",
					},
				},
			},
		},
	}

	application.SetGroupVersionKind(argoCDApplicationGVK)

	// Check if application exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(argoCDApplicationGVK)
	err := r.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ArgoCD component application", "name", appName)
			return r.Create(ctx, application)
		}
		return err
	}

	// Update if exists
	application.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, application)
}

// Create App-of-Apps pattern for managing all applications
func (r *ApplicationClaimReconciler) createAppOfApps(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)
	appName := fmt.Sprintf("%s-umbrella", claim.Spec.Namespace)

	// Git repository for app-of-apps configuration
	repoURL := os.Getenv("GIT_REPO_URL")
	if repoURL == "" {
		repoURL = "https://github.com/infraforge/platform-configs"
	}

	// Path to app-of-apps configuration
	path := fmt.Sprintf("teams/%s/environments/%s/app-of-apps",
		strings.ToLower(strings.ReplaceAll(claim.Spec.Owner.Team, " ", "-")),
		claim.Spec.Environment)

	// Create ArgoCD Application for app-of-apps
	application := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      appName,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed": "true",
					"platform.infraforge.io/team":    claim.Spec.Owner.Team,
					"platform.infraforge.io/env":     claim.Spec.Environment,
					"platform.infraforge.io/type":    "app-of-apps",
				},
				"finalizers": []string{
					"resources-finalizer.argocd.argoproj.io",
				},
			},
			"spec": map[string]interface{}{
				"project": fmt.Sprintf("%s-%s", claim.Spec.Owner.Team, claim.Spec.Environment),
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"path":           path,
					"targetRevision": "HEAD",
					"directory": map[string]interface{}{
						"recurse": true,
						"jsonnet": map[string]interface{}{},
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": "argocd",
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
					"syncOptions": []string{
						"CreateNamespace=true",
					},
				},
			},
		},
	}

	application.SetGroupVersionKind(argoCDApplicationGVK)

	// Check if application exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(argoCDApplicationGVK)
	err := r.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ArgoCD app-of-apps", "name", appName)
			return r.Create(ctx, application)
		}
		return err
	}

	// Update if exists
	application.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, application)
}

// Build Helm parameters for application
func (r *ApplicationClaimReconciler) buildHelmParameters(app platformv1.ApplicationSpec, claim *platformv1.ApplicationClaim) []map[string]interface{} {
	params := []map[string]interface{}{
		{
			"name":  "replicaCount",
			"value": fmt.Sprintf("%d", app.Replicas),
		},
		{
			"name":  "image.tag",
			"value": app.Version,
		},
		{
			"name":  "environment",
			"value": claim.Spec.Environment,
		},
		{
			"name":  "team",
			"value": claim.Spec.Owner.Team,
		},
	}

	// Add environment variables
	for _, env := range app.Env {
		params = append(params, map[string]interface{}{
			"name":  fmt.Sprintf("env.%s", env.Name),
			"value": env.Value,
		})
	}

	// Add ports
	for i, port := range app.Ports {
		params = append(params, map[string]interface{}{
			"name":  fmt.Sprintf("service.ports[%d].name", i),
			"value": port.Name,
		})
		params = append(params, map[string]interface{}{
			"name":  fmt.Sprintf("service.ports[%d].port", i),
			"value": fmt.Sprintf("%d", port.Port),
		})
	}

	return params
}

// Build values for component
func (r *ApplicationClaimReconciler) buildComponentValues(component platformv1.ComponentSpec, claim *platformv1.ApplicationClaim) string {
	// Build YAML values for the component
	values := fmt.Sprintf(`
global:
  storageClass: standard

auth:
  enabled: true
  database: %s
  username: %s

persistence:
  enabled: true
  size: 10Gi

metrics:
  enabled: true
  serviceMonitor:
    enabled: true

resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "1Gi"
    cpu: "1000m"

labels:
  "platform.infraforge.io/managed": "true"
  "platform.infraforge.io/team": "%s"
  "platform.infraforge.io/component": "%s"
`, component.Name, component.Name, claim.Spec.Owner.Team, component.Type)

	return values
}

// createIndividualApplication creates an ArgoCD Application for a single app or component
func (r *ApplicationClaimReconciler) createIndividualApplication(ctx context.Context, claim *platformv1.ApplicationClaim, appName, projectName string) error {
	logger := log.FromContext(ctx)

	// Read values from ConfigMap
	valuesYAML, err := r.getValuesYAMLFromConfigMap(ctx, claim.Name, appName)
	if err != nil {
		return fmt.Errorf("failed to get values for %s: %w", appName, err)
	}

	teamName := normalizeK8sName(claim.Spec.Owner.Team)
	applicationName := fmt.Sprintf("%s-%s", claim.Name, appName)

	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      applicationName,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed": "true",
					"platform.infraforge.io/claim":   claim.Name,
					"platform.infraforge.io/team":    teamName,
					"platform.infraforge.io/env":     claim.Spec.Environment,
				},
			},
			"spec": map[string]interface{}{
				"project": projectName,
				"source": map[string]interface{}{
					"repoURL":        "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
					"targetRevision": "2.0.0",
					"chart":          "common",
					"helm": map[string]interface{}{
						"values": valuesYAML,
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": claim.Spec.Namespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
					"syncOptions": []string{
						"CreateNamespace=true",
					},
				},
			},
		},
	}

	app.SetGroupVersionKind(argoCDApplicationGVK)

	// Check if Application exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(argoCDApplicationGVK)

	err = r.Get(ctx, types.NamespacedName{
		Name:      applicationName,
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Application
			logger.Info("Creating ArgoCD Application", "name", applicationName)
			if err := r.Create(ctx, app); err != nil {
				return fmt.Errorf("failed to create Application: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get Application: %w", err)
	}

	// Update existing Application
	logger.Info("Updating ArgoCD Application", "name", applicationName)
	existing.Object["spec"] = app.Object["spec"]
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update Application: %w", err)
	}

	return nil
}

// createApplicationSet creates an ApplicationSet for all apps/components in a claim
func (r *ApplicationClaimReconciler) createApplicationSet(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)

	teamName := normalizeK8sName(claim.Spec.Owner.Team)
	projectName := fmt.Sprintf("%s-%s", teamName, claim.Spec.Environment)

	// ApplicationSet name: {env}-{claim-name}-appset
	appSetName := fmt.Sprintf("%s-%s-appset", claim.Spec.Environment, claim.Name)

	// Build list of all apps and components
	var elements []map[string]interface{}

	// Add applications
	for _, app := range claim.Spec.Applications {
		valuesYAML, err := r.generateValuesForApp(claim, app)
		if err != nil {
			return fmt.Errorf("failed to generate values for app %s: %w", app.Name, err)
		}

		elements = append(elements, map[string]interface{}{
			"name":       app.Name,
			"type":       "app",
			"helmValues": valuesYAML,
		})
	}

	// Add components
	for _, component := range claim.Spec.Components {
		valuesYAML, err := r.generateValuesForComponent(claim, component)
		if err != nil {
			return fmt.Errorf("failed to generate values for component %s: %w", component.Name, err)
		}

		elements = append(elements, map[string]interface{}{
			"name":       component.Name,
			"type":       "component",
			"helmValues": valuesYAML,
		})
	}

	// Build ApplicationSet
	appSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      appSetName,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed":     "true",
					"platform.infraforge.io/claim":       claim.Name,
					"platform.infraforge.io/team":        teamName,
					"platform.infraforge.io/environment": claim.Spec.Environment,
				},
			},
			"spec": map[string]interface{}{
				"generators": []map[string]interface{}{
					{
						"list": map[string]interface{}{
							"elements": elements,
						},
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": fmt.Sprintf("%s-{{name}}", claim.Name),
						"labels": map[string]interface{}{
							"platform.infraforge.io/managed":     "true",
							"platform.infraforge.io/claim":       claim.Name,
							"platform.infraforge.io/team":        teamName,
							"platform.infraforge.io/environment": claim.Spec.Environment,
						},
					},
					"spec": map[string]interface{}{
						"project": projectName,
						"source": map[string]interface{}{
							"repoURL":        "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
							"targetRevision": "2.0.0",
							"chart":          "common",
							"helm": map[string]interface{}{
								// Values embedded inline from list generator
								"values": "{{helmValues}}",
							},
						},
						"destination": map[string]interface{}{
							"server":    "https://kubernetes.default.svc",
							"namespace": claim.Spec.Namespace,
						},
						"syncPolicy": map[string]interface{}{
							"automated": map[string]interface{}{
								"prune":    true,
								"selfHeal": true,
							},
							"syncOptions": []string{
								"CreateNamespace=true",
							},
						},
					},
				},
			},
		},
	}

	appSet.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "ApplicationSet",
	})

	// Check if ApplicationSet exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "ApplicationSet",
	})

	err := r.Get(ctx, types.NamespacedName{
		Name:      appSetName,
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ApplicationSet
			logger.Info("Creating ArgoCD ApplicationSet", "name", appSetName)
			if err := r.Create(ctx, appSet); err != nil {
				return fmt.Errorf("failed to create ApplicationSet: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get ApplicationSet: %w", err)
	}

	// Update existing ApplicationSet
	logger.Info("Updating ArgoCD ApplicationSet", "name", appSetName)
	existing.Object["spec"] = appSet.Object["spec"]
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ApplicationSet: %w", err)
	}

	return nil
}

// getValuesYAMLFromConfigMap retrieves Helm values YAML from ConfigMap
func (r *ApplicationClaimReconciler) getValuesYAMLFromConfigMap(ctx context.Context, claimName, appName string) (string, error) {
	configMapName := fmt.Sprintf("%s-%s-values", claimName, appName)

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: "argocd",
	}, configMap)

	if err != nil {
		return "", fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	valuesYAML, ok := configMap.Data["values.yaml"]
	if !ok {
		return "", fmt.Errorf("values.yaml not found in ConfigMap %s", configMapName)
	}

	return valuesYAML, nil
}
