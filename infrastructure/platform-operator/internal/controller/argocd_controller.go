package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "platform.infraforge.io/platform-operator/api/v1"
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

	// 3. Create ArgoCD Applications for each application
	for _, app := range claim.Spec.Applications {
		if err := r.createArgoCDApplication(ctx, claim, app); err != nil {
			logger.Error(err, "Failed to create ArgoCD application", "app", app.Name)
			return err
		}
	}

	// 4. Create ArgoCD Applications for components (PostgreSQL, Redis, etc.)
	for _, component := range claim.Spec.Components {
		if err := r.createArgoCDComponentApplication(ctx, claim, component); err != nil {
			logger.Error(err, "Failed to create ArgoCD component application", "component", component.Name)
			return err
		}
	}

	// 5. Create ArgoCD App-of-Apps pattern for managing everything
	if err := r.createAppOfApps(ctx, claim); err != nil {
		return fmt.Errorf("failed to create app-of-apps: %w", err)
	}

	return nil
}

// Create ArgoCD Project for team isolation
func (r *ApplicationClaimReconciler) createArgoCDProject(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)

	project := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "AppProject",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-%s", claim.Spec.Owner.Team, claim.Spec.Environment),
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed": "true",
					"platform.infraforge.io/team":    claim.Spec.Owner.Team,
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
						"prune":    true,
						"selfHeal": true,
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
					"platform.infraforge.io/managed":    "true",
					"platform.infraforge.io/team":       claim.Spec.Owner.Team,
					"platform.infraforge.io/env":        claim.Spec.Environment,
					"platform.infraforge.io/type":       "app-of-apps",
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
			"value": fmt.Sprintf("%d", *app.Replicas),
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