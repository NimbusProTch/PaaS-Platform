package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"github.com/infraforge/platform-operator/pkg/gitea"
)

// ApplicationClaimGitOpsReconciler reconciles ApplicationClaim with GitOps
type ApplicationClaimGitOpsReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Gitea credentials - client created dynamically from claim
	GiteaUsername string
	GiteaToken    string
	VoltranRepo   string
	Branch        string
}

//+kubebuilder:rbac:groups=platform.infraforge.io,resources=applicationclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=applicationclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=applicationclaims/finalizers,verbs=update

// Reconcile handles ApplicationClaim reconciliation with GitOps
func (r *ApplicationClaimGitOpsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ApplicationClaim (GitOps)", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ApplicationClaim
	claim := &platformv1.ApplicationClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch ApplicationClaim")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if claim.Status.Phase == "" {
		claim.Status.Phase = "Pending"
		claim.Status.LastUpdated = metav1.Now()
		if err := r.Status().Update(ctx, claim); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Always reconcile to handle spec changes
	// This ensures updates to the ApplicationClaim are always processed

	// Create GiteaClient dynamically from claim
	giteaClient := gitea.NewClient(claim.Spec.GiteaURL, r.GiteaUsername, r.GiteaToken)

	// Generate ApplicationSet and values.yaml
	logger.Info("Generating ApplicationSet and values", "environment", claim.Spec.Environment)

	files := make(map[string]string)

	// Generate ApplicationSet
	appSetPath := fmt.Sprintf("appsets/%s/apps/%s-appset.yaml", claim.Spec.ClusterType, claim.Spec.Environment)
	appSetContent := r.generateApplicationSet(claim)
	files[appSetPath] = appSetContent
	logger.Info("Generated ApplicationSet content", "path", appSetPath, "length", len(appSetContent))

	// Generate directory structure for each application
	enabledCount := 0
	for _, app := range claim.Spec.Applications {
		// Skip disabled applications
		if !app.Enabled {
			logger.Info("Skipping disabled application", "name", app.Name)
			continue
		}
		enabledCount++

		// values.yaml
		valuesPath := fmt.Sprintf("environments/%s/%s/applications/%s/values.yaml", claim.Spec.ClusterType, claim.Spec.Environment, app.Name)
		valuesContent := r.generateValuesYAML(claim, app)
		files[valuesPath] = valuesContent

		// config.json (metadata for ApplicationSet)
		configPath := fmt.Sprintf("environments/%s/%s/applications/%s/config.json", claim.Spec.ClusterType, claim.Spec.Environment, app.Name)
		configContent := r.generateConfigJSON(claim, app)
		files[configPath] = configContent

		logger.Info("Generated application files", "app", app.Name, "valuesPath", valuesPath, "configPath", configPath)
	}

	logger.Info("Total files to push", "fileCount", len(files), "enabledApps", enabledCount)

	// Push to Gitea - use internal clone URL
	voltranURL := giteaClient.ConstructCloneURL(claim.Spec.Organization, r.VoltranRepo)
	commitMsg := fmt.Sprintf("Update %s environment applications by operator", claim.Spec.Environment)

	logger.Info("Pushing files to Gitea", "url", voltranURL, "branch", r.Branch, "commitMsg", commitMsg)

	if err := giteaClient.PushFiles(ctx, voltranURL, r.Branch, files, commitMsg,
		"Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push to Git", "url", voltranURL)
		// Don't update status on git errors, just retry
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("Successfully pushed files to Git")

	// Create ApplicationSet in ArgoCD namespace
	logger.Info("Creating ApplicationSet in ArgoCD", "name", fmt.Sprintf("%s-apps", claim.Spec.Environment))
	appSet := r.generateApplicationSet(claim)
	if err := r.createApplicationSet(ctx, appSet, claim); err != nil {
		logger.Error(err, "failed to create ApplicationSet")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("Successfully created ApplicationSet")

	// Update status to Ready only if not already ready
	if claim.Status.Phase != "Ready" || !claim.Status.Ready {
		claim.Status.Phase = "Ready"
		claim.Status.Ready = true
		claim.Status.ApplicationsReady = true
		claim.Status.LastUpdated = metav1.Now()
		if err := r.Status().Update(ctx, claim); err != nil {
			logger.Error(err, "failed to update status")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	logger.Info("ApplicationClaim reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// generateApplicationSet generates ArgoCD ApplicationSet manifest
func (r *ApplicationClaimGitOpsReconciler) generateApplicationSet(claim *platformv1.ApplicationClaim) string {
	appSet := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "ApplicationSet",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-apps", claim.Spec.Environment),
			"namespace": "argocd",
			"labels": map[string]string{
				"platform.infraforge.io/environment": claim.Spec.Environment,
				"platform.infraforge.io/cluster":     claim.Spec.ClusterType,
			},
		},
		"spec": map[string]interface{}{
			"generators": []map[string]interface{}{
				{
					"list": map[string]interface{}{
						"elements": r.generateApplicationElements(claim),
					},
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "{{name}}-" + claim.Spec.Environment,
					"labels": map[string]string{
						"platform.infraforge.io/app": "{{name}}",
						"platform.infraforge.io/env": claim.Spec.Environment,
					},
				},
				"spec": map[string]interface{}{
					"project": "default",
					"source": map[string]interface{}{
						"repoURL":        "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
						"chart":          "{{chart}}",
						"targetRevision": "{{version}}",
						"helm": map[string]interface{}{
							"values": "{{values}}",
						},
					},
					"destination": map[string]interface{}{
						"server":    "https://kubernetes.default.svc",
						"namespace": claim.Spec.Environment,
					},
					"syncPolicy": map[string]interface{}{
						"automated": map[string]interface{}{
							"prune":    true,
							"selfHeal": true,
						},
						"syncOptions": []string{"CreateNamespace=true"},
					},
				},
			},
		},
	}

	data, _ := yaml.Marshal(appSet)
	return string(data)
}

// generateApplicationElements generates list elements for ApplicationSet
func (r *ApplicationClaimGitOpsReconciler) generateApplicationElements(claim *platformv1.ApplicationClaim) []map[string]string {
	elements := []map[string]string{}

	for _, app := range claim.Spec.Applications {
		// Skip disabled applications
		if !app.Enabled {
			continue
		}

		chartName := app.Chart.Name
		if chartName == "" {
			chartName = "microservice" // default chart name
		}

		version := app.Chart.Version
		if version == "" {
			version = "1.0.0" // default version
		}

		valuesYAML := r.generateValuesYAML(claim, app)

		elements = append(elements, map[string]string{
			"name":    app.Name,
			"chart":   chartName,
			"version": version,
			"values":  valuesYAML,
		})
	}

	return elements
}

// generateConfigJSON generates config.json with chart metadata and service name
func (r *ApplicationClaimGitOpsReconciler) generateConfigJSON(claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) string {
	config := map[string]interface{}{
		"name":    app.Name,
		"chart":   app.Chart.Name,  // Just chart name, no prefix
		"version": app.Chart.Version,
		"values":  r.generateValuesYAML(claim, app),
	}

	if app.Chart.Version == "" {
		config["version"] = "1.0.0"
	}

	jsonBytes, _ := json.Marshal(config)
	return string(jsonBytes)
}

// generateValuesYAML generates Helm values.yaml for an application
// Since charts are now in Gitea, we just generate values from CRD spec
func (r *ApplicationClaimGitOpsReconciler) generateValuesYAML(claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) string {
	// Generate values from CRD spec
	return r.generateValuesFromCRD(claim, app)
}

// generateValuesFromCRD generates values.yaml from CRD spec only (fallback)
func (r *ApplicationClaimGitOpsReconciler) generateValuesFromCRD(claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) string {
	values := r.buildCRDOverrides(app)
	data, _ := yaml.Marshal(values)
	return string(data)
}

// buildCRDOverrides builds override values from CRD spec
func (r *ApplicationClaimGitOpsReconciler) buildCRDOverrides(app platformv1.ApplicationSpec) map[string]interface{} {
	overrides := make(map[string]interface{})

	// Image configuration
	if app.Image.Repository != "" {
		overrides["image"] = map[string]interface{}{
			"repository": app.Image.Repository,
			"tag":        app.Image.Tag,
		}
		if app.Image.PullPolicy != "" {
			overrides["image"].(map[string]interface{})["pullPolicy"] = app.Image.PullPolicy
		}
	}

	// Always add imagePullSecrets for GHCR
	overrides["imagePullSecrets"] = []map[string]interface{}{
		{
			"name": "ghcr-pull-secret",
		},
	}

	// Replica count
	if app.Replicas > 0 {
		overrides["replicaCount"] = app.Replicas
	}

	// Resources
	if app.Resources.Requests.CPU != "" || app.Resources.Requests.Memory != "" ||
		app.Resources.Limits.CPU != "" || app.Resources.Limits.Memory != "" {
		resources := map[string]interface{}{}
		if app.Resources.Requests.CPU != "" || app.Resources.Requests.Memory != "" {
			resources["requests"] = map[string]interface{}{}
			if app.Resources.Requests.CPU != "" {
				resources["requests"].(map[string]interface{})["cpu"] = app.Resources.Requests.CPU
			}
			if app.Resources.Requests.Memory != "" {
				resources["requests"].(map[string]interface{})["memory"] = app.Resources.Requests.Memory
			}
		}
		if app.Resources.Limits.CPU != "" || app.Resources.Limits.Memory != "" {
			resources["limits"] = map[string]interface{}{}
			if app.Resources.Limits.CPU != "" {
				resources["limits"].(map[string]interface{})["cpu"] = app.Resources.Limits.CPU
			}
			if app.Resources.Limits.Memory != "" {
				resources["limits"].(map[string]interface{})["memory"] = app.Resources.Limits.Memory
			}
		}
		overrides["resources"] = resources
	}

	// Ingress
	if app.Ingress != nil && app.Ingress.Enabled {
		ingress := map[string]interface{}{
			"enabled": true,
		}
		if app.Ingress.Host != "" {
			ingress["host"] = app.Ingress.Host
		}
		if app.Ingress.Path != "" {
			ingress["path"] = app.Ingress.Path
		}
		if app.Ingress.TLS {
			ingress["tls"] = true
		}
		if len(app.Ingress.Annotations) > 0 {
			ingress["annotations"] = app.Ingress.Annotations
		}
		overrides["ingress"] = ingress
	}

	// Environment variables
	if len(app.Env) > 0 {
		envVars := []map[string]interface{}{}
		for _, env := range app.Env {
			envVar := map[string]interface{}{
				"name": env.Name,
			}
			if env.Value != "" {
				envVar["value"] = env.Value
			}
			envVars = append(envVars, envVar)
		}
		overrides["env"] = envVars
	}

	// Autoscaling
	if app.Autoscaling != nil && app.Autoscaling.Enabled {
		autoscaling := map[string]interface{}{
			"enabled": true,
		}
		if app.Autoscaling.MinReplicas > 0 {
			autoscaling["minReplicas"] = app.Autoscaling.MinReplicas
		}
		if app.Autoscaling.MaxReplicas > 0 {
			autoscaling["maxReplicas"] = app.Autoscaling.MaxReplicas
		}
		if app.Autoscaling.TargetCPUUtilizationPercentage > 0 {
			autoscaling["targetCPUUtilizationPercentage"] = app.Autoscaling.TargetCPUUtilizationPercentage
		}
		if app.Autoscaling.TargetMemoryUtilizationPercentage > 0 {
			autoscaling["targetMemoryUtilizationPercentage"] = app.Autoscaling.TargetMemoryUtilizationPercentage
		}
		overrides["autoscaling"] = autoscaling
	}

	return overrides
}

// createApplicationSet creates or updates the ApplicationSet in ArgoCD namespace
func (r *ApplicationClaimGitOpsReconciler) createApplicationSet(ctx context.Context, appSetYAML string, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting createApplicationSet", "yamlLength", len(appSetYAML))

	// Parse YAML to unstructured object
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(appSetYAML), &obj.Object); err != nil {
		logger.Error(err, "Failed to unmarshal ApplicationSet YAML")
		return fmt.Errorf("failed to unmarshal ApplicationSet: %w", err)
	}

	// Set namespace to argocd
	obj.SetNamespace("argocd")
	appSetName := obj.GetName()
	logger.Info("ApplicationSet details", "name", appSetName, "namespace", "argocd", "kind", obj.GetKind())

	// Create or update ApplicationSet
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ApplicationSet
			logger.Info("ApplicationSet does not exist, creating new one", "name", appSetName)
			if err := r.Create(ctx, obj); err != nil {
				logger.Error(err, "Failed to create ApplicationSet", "name", appSetName)
				return fmt.Errorf("failed to create ApplicationSet: %w", err)
			}
			logger.Info("Successfully created ApplicationSet", "name", appSetName)
		} else {
			logger.Error(err, "Failed to get ApplicationSet", "name", appSetName)
			return fmt.Errorf("failed to get ApplicationSet: %w", err)
		}
	} else {
		// Update existing ApplicationSet
		logger.Info("ApplicationSet already exists, updating", "name", appSetName)
		obj.SetResourceVersion(existing.GetResourceVersion())
		if err := r.Update(ctx, obj); err != nil {
			logger.Error(err, "Failed to update ApplicationSet", "name", appSetName)
			return fmt.Errorf("failed to update ApplicationSet: %w", err)
		}
		logger.Info("Successfully updated ApplicationSet", "name", appSetName)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ApplicationClaimGitOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.ApplicationClaim{}).
		Complete(r)
}
