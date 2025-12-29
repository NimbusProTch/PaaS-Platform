package controller

import (
	"context"
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

// PlatformApplicationClaimReconciler reconciles a PlatformApplicationClaim object
type PlatformApplicationClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Gitea credentials - client created dynamically from claim
	GiteaUsername string
	GiteaToken    string
	VoltranRepo   string
	Branch        string
}

//+kubebuilder:rbac:groups=platform.infraforge.io,resources=platformapplicationclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=platformapplicationclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=platformapplicationclaims/finalizers,verbs=update

// Reconcile handles PlatformApplicationClaim reconciliation
// This will process platform services like PostgreSQL, Redis, RabbitMQ, etc.
func (r *PlatformApplicationClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling PlatformApplicationClaim", "name", req.Name, "namespace", req.Namespace)

	// Fetch the PlatformApplicationClaim
	claim := &platformv1.PlatformApplicationClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch PlatformApplicationClaim")
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

	// Skip if already ready
	if claim.Status.Phase == "Ready" && claim.Status.Ready {
		return ctrl.Result{}, nil
	}

	// Create GiteaClient dynamically from claim
	giteaClient := gitea.NewClient(claim.Spec.GiteaURL, r.GiteaUsername, r.GiteaToken)

	// Generate ApplicationSet and values.yaml for platform services
	logger.Info("Generating platform ApplicationSet and values", "environment", claim.Spec.Environment)

	files := make(map[string]string)

	// Generate ApplicationSet for platform services
	appSetPath := fmt.Sprintf("appsets/%s/platform/%s-platform-appset.yaml", claim.Spec.ClusterType, claim.Spec.Environment)
	appSetContent := r.generatePlatformApplicationSet(claim, giteaClient)
	files[appSetPath] = appSetContent
	logger.Info("Generated platform ApplicationSet content", "path", appSetPath, "length", len(appSetContent))

	// Generate values.yaml for each service
	enabledCount := 0
	for _, service := range claim.Spec.Services {
		// Skip disabled services
		if !service.Enabled {
			logger.Info("Skipping disabled platform service", "name", service.Name)
			continue
		}
		enabledCount++

		valuesPath := fmt.Sprintf("environments/%s/%s/platform/%s/values.yaml", claim.Spec.ClusterType, claim.Spec.Environment, service.Name)
		valuesContent := r.generatePlatformValuesYAML(claim, service, giteaClient)
		files[valuesPath] = valuesContent
		logger.Info("Generated platform service files", "service", service.Name, "valuesPath", valuesPath)
	}

	logger.Info("Total platform files to push", "fileCount", len(files), "enabledServices", enabledCount)

	// Push to Gitea - use internal clone URL
	voltranURL := giteaClient.ConstructCloneURL(claim.Spec.Organization, r.VoltranRepo)
	commitMsg := fmt.Sprintf("Update %s environment platform services by operator", claim.Spec.Environment)

	logger.Info("Pushing platform files to Gitea", "url", voltranURL, "branch", r.Branch, "commitMsg", commitMsg)

	if err := giteaClient.PushFiles(ctx, voltranURL, r.Branch, files, commitMsg,
		"Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push to Git", "url", voltranURL)
		// Don't update status on git errors, just retry
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("Successfully pushed platform files to Git")

	// Create individual Applications in ArgoCD namespace for platform services
	logger.Info("Creating Applications in ArgoCD for platform services")
	for _, service := range claim.Spec.Services {
		if !service.Enabled {
			continue
		}

		appManifest := r.generatePlatformApplication(claim, service)
		if err := r.createApplication(ctx, appManifest); err != nil {
			logger.Error(err, "failed to create Application", "service", service.Name)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		logger.Info("Created Application", "name", service.Name)
	}

	// Update status to Ready only if not already ready
	if claim.Status.Phase != "Ready" || !claim.Status.Ready {
		claim.Status.Phase = "Ready"
		claim.Status.Ready = true
		claim.Status.ServicesReady = true
		claim.Status.LastUpdated = metav1.Now()
		if err := r.Status().Update(ctx, claim); err != nil {
			logger.Error(err, "failed to update status")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	logger.Info("PlatformApplicationClaim reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// generatePlatformApplication generates a simple ArgoCD Application manifest for platform services
func (r *PlatformApplicationClaimReconciler) generatePlatformApplication(claim *platformv1.PlatformApplicationClaim, service platformv1.PlatformServiceSpec) string {
	chartName := service.Chart.Name
	if chartName == "" {
		chartName = service.Type // fallback to type
	}

	version := service.Chart.Version
	if version == "" {
		version = "1.0.0"
	}

	// Parse custom values from CRD
	var customValues map[string]interface{}
	if service.Values.Raw != nil {
		if err := yaml.Unmarshal(service.Values.Raw, &customValues); err != nil {
			customValues = make(map[string]interface{})
		}
	} else {
		customValues = make(map[string]interface{})
	}

	valuesYAML, _ := yaml.Marshal(customValues)

	application := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-%s", service.Name, claim.Spec.Environment),
			"namespace": "argocd",
			"labels": map[string]string{
				"platform.infraforge.io/service":     service.Name,
				"platform.infraforge.io/environment": claim.Spec.Environment,
				"platform.infraforge.io/type":        "platform",
			},
		},
		"spec": map[string]interface{}{
			"project": "default",
			"source": map[string]interface{}{
				"repoURL":        "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
				"chart":          chartName,
				"targetRevision": version,
				"helm": map[string]interface{}{
					"values": string(valuesYAML),
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
	}

	data, _ := yaml.Marshal(application)
	return string(data)
}

// createApplication creates or updates an Application in ArgoCD namespace
func (r *PlatformApplicationClaimReconciler) createApplication(ctx context.Context, appYAML string) error {
	logger := log.FromContext(ctx)

	// Parse YAML to unstructured object
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(appYAML), &obj.Object); err != nil {
		return fmt.Errorf("failed to unmarshal Application: %w", err)
	}

	// Set namespace to argocd
	obj.SetNamespace("argocd")
	appName := obj.GetName()

	// Create or update Application
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Application
			if err := r.Create(ctx, obj); err != nil {
				return fmt.Errorf("failed to create Application: %w", err)
			}
			logger.Info("Successfully created Application", "name", appName)
		} else {
			return fmt.Errorf("failed to get Application: %w", err)
		}
	} else {
		// Update existing Application
		obj.SetResourceVersion(existing.GetResourceVersion())
		if err := r.Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to update Application: %w", err)
		}
		logger.Info("Successfully updated Application", "name", appName)
	}

	return nil
}

// generatePlatformApplicationSet generates ArgoCD ApplicationSet for platform services
func (r *PlatformApplicationClaimReconciler) generatePlatformApplicationSet(claim *platformv1.PlatformApplicationClaim, giteaClient *gitea.Client) string {
	// Use Git Directories Generator to read from pushed values files
	appSet := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "ApplicationSet",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-platform", claim.Spec.Environment),
			"namespace": "argocd",
			"labels": map[string]string{
				"platform.infraforge.io/environment": claim.Spec.Environment,
				"platform.infraforge.io/cluster":     claim.Spec.ClusterType,
				"platform.infraforge.io/type":        "platform",
			},
		},
		"spec": map[string]interface{}{
			"generators": []map[string]interface{}{
				{
					"git": map[string]interface{}{
						"repoURL":  giteaClient.ConstructCloneURL(claim.Spec.Organization, r.VoltranRepo),
						"revision": r.Branch,
						"directories": []map[string]interface{}{
							{
								"path": fmt.Sprintf("environments/%s/%s/platform/*",
									claim.Spec.ClusterType, claim.Spec.Environment),
							},
						},
					},
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("{{path.basename}}-%s", claim.Spec.Environment),
					"labels": map[string]string{
						"platform.infraforge.io/service": "{{path.basename}}",
						"platform.infraforge.io/env":     claim.Spec.Environment,
						"platform.infraforge.io/type":    "platform",
					},
				},
				"spec": map[string]interface{}{
					"project": "default",
					"source": map[string]interface{}{
						"repoURL":        "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
						"chart":          "{{chart | default path.basename}}",
						"targetRevision": "{{version | default \"1.0.0\"}}",
						"helm": map[string]interface{}{
							"valueFiles": []string{
								fmt.Sprintf("https://gitea-http.gitea.svc.cluster.local:3000/infraforge/voltran/raw/branch/main/environments/%s/%s/platform/{{path.basename}}/values.yaml",
									claim.Spec.ClusterType, claim.Spec.Environment),
							},
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


// generatePlatformValuesYAML generates Helm values.yaml for a platform service
// Since charts are now in Gitea, we just generate values from CRD spec
func (r *PlatformApplicationClaimReconciler) generatePlatformValuesYAML(claim *platformv1.PlatformApplicationClaim, service platformv1.PlatformServiceSpec, giteaClient *gitea.Client) string {
	logger := log.Log.WithName("generatePlatformValuesYAML")

	// Parse custom values from CRD
	var customValues map[string]interface{}
	if service.Values.Raw != nil {
		if err := yaml.Unmarshal(service.Values.Raw, &customValues); err != nil {
			logger.Error(err, "failed to parse custom values, using empty", "service", service.Name)
			customValues = make(map[string]interface{})
		}
	}

	if customValues == nil {
		customValues = make(map[string]interface{})
	}

	data, _ := yaml.Marshal(customValues)
	return string(data)
}

// createApplicationSet creates or updates the ApplicationSet in ArgoCD namespace
func (r *PlatformApplicationClaimReconciler) createApplicationSet(ctx context.Context, appSetYAML string, claim *platformv1.PlatformApplicationClaim) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting createApplicationSet for platform", "yamlLength", len(appSetYAML))

	// Parse YAML to unstructured object
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(appSetYAML), &obj.Object); err != nil {
		logger.Error(err, "Failed to unmarshal platform ApplicationSet YAML")
		return fmt.Errorf("failed to unmarshal ApplicationSet: %w", err)
	}

	// Set namespace to argocd
	obj.SetNamespace("argocd")
	appSetName := obj.GetName()
	logger.Info("Platform ApplicationSet details", "name", appSetName, "namespace", "argocd", "kind", obj.GetKind())

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
			logger.Info("Platform ApplicationSet does not exist, creating new one", "name", appSetName)
			if err := r.Create(ctx, obj); err != nil {
				logger.Error(err, "Failed to create platform ApplicationSet", "name", appSetName)
				return fmt.Errorf("failed to create ApplicationSet: %w", err)
			}
			logger.Info("Successfully created platform ApplicationSet", "name", appSetName)
		} else {
			logger.Error(err, "Failed to get platform ApplicationSet", "name", appSetName)
			return fmt.Errorf("failed to get ApplicationSet: %w", err)
		}
	} else {
		// Update existing ApplicationSet
		logger.Info("Platform ApplicationSet already exists, updating", "name", appSetName)
		obj.SetResourceVersion(existing.GetResourceVersion())
		if err := r.Update(ctx, obj); err != nil {
			logger.Error(err, "Failed to update platform ApplicationSet", "name", appSetName)
			return fmt.Errorf("failed to update ApplicationSet: %w", err)
		}
		logger.Info("Successfully updated platform ApplicationSet", "name", appSetName)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *PlatformApplicationClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.PlatformApplicationClaim{}).
		Complete(r)
}
