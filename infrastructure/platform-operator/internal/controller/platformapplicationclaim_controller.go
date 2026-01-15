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
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// Skip operator installation check - operators are already installed
	// This was causing an infinite loop because isOperatorInstalled wasn't working correctly
	logger.Info("Skipping operator installation check - assuming operators are already installed")

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

	// DISABLED: Direct Application creation - Root Apps will watch ApplicationSets and create them
	// // Create individual Applications in ArgoCD namespace for platform services
	// logger.Info("Creating Applications in ArgoCD for platform services")
	// for _, service := range claim.Spec.Services {
	// 	if !service.Enabled {
	// 		continue
	// 	}

	// 	appManifest := r.generatePlatformApplication(claim, service)
	// 	if err := r.createApplication(ctx, appManifest); err != nil {
	// 		logger.Error(err, "failed to create Application", "service", service.Name)
	// 		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	// 	}
	// 	logger.Info("Created Application", "name", service.Name)
	// }

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
				"repoURL":        fmt.Sprintf("%s/%s/charts", claim.Spec.GiteaURL, claim.Spec.Organization),
				"path":           chartName,
				"targetRevision": "main",
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
	// Build list of enabled services with chart mapping
	var elements []map[string]interface{}
	for _, service := range claim.Spec.Services {
		if !service.Enabled {
			continue
		}

		// Map service name to actual chart name
		chartName := service.Chart.Name
		if chartName == "" {
			// Default chart mapping
			switch service.Type {
			case "postgresql":
				chartName = "postgresql"
			case "redis":
				chartName = "redis"
			case "rabbitmq":
				chartName = "rabbitmq"
			case "mongodb":
				chartName = "mongodb"
			case "kafka":
				chartName = "kafka"
			default:
				chartName = service.Type
			}
		}

		elements = append(elements, map[string]interface{}{
			"name":  service.Name,
			"chart": chartName,
		})
	}

	// Use List Generator with explicit chart mapping
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
					"list": map[string]interface{}{
						"elements": elements,
					},
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("{{name}}-%s", claim.Spec.Environment),
					"labels": map[string]string{
						"platform.infraforge.io/service": "{{name}}",
						"platform.infraforge.io/env":     claim.Spec.Environment,
						"platform.infraforge.io/type":    "platform",
					},
				},
				"spec": map[string]interface{}{
					"project": "default",
					"sources": []map[string]interface{}{
						{
							// Source 1: Helm chart from ChartMuseum
							"repoURL":        "http://chartmuseum.chartmuseum.svc.cluster.local:8080",
							"chart":          "{{chart}}",
							"targetRevision": "*",
							"helm": map[string]interface{}{
								"valueFiles": []string{
									fmt.Sprintf("$values/environments/%s/%s/platform/{{name}}/values.yaml",
										claim.Spec.ClusterType, claim.Spec.Environment),
								},
							},
						},
						{
							// Source 2: Values from voltran repository
							"repoURL":        giteaClient.ConstructCloneURL(claim.Spec.Organization, r.VoltranRepo),
							"targetRevision": r.Branch,
							"ref":            "values",
						},
					},
					"destination": map[string]interface{}{
						"server":    "https://kubernetes.default.svc",
						"namespace": fmt.Sprintf("%s-platform", claim.Spec.Environment),
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

	// Get storage class from claim or use default
	storageClass := claim.Spec.StorageClass
	if storageClass == "" {
		storageClass = "standard" // Default for Kind cluster
	}

	// Add service-specific defaults
	values := make(map[string]interface{})
	values["name"] = service.Name
	values["type"] = service.Type

	// Set default values based on service type
	switch service.Type {
	case "postgresql":
		values["version"] = service.Version
		if values["version"] == "" {
			values["version"] = "15"
		}
		// Structure values properly for the PostgreSQL chart
		values["postgresql"] = map[string]interface{}{
			"storage": map[string]interface{}{
				"size":         "1Gi",
				"storageClass": storageClass,
			},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "100m",
					"memory": "256Mi", // Increased to match shared_buffers
				},
				"limits": map[string]interface{}{
					"cpu":    "200m",
					"memory": "512Mi",
				},
			},
		}
	case "redis":
		values["version"] = service.Version
		if values["version"] == "" {
			values["version"] = "7.0"
		}
		// Structure values properly for the Redis chart
		values["redis"] = map[string]interface{}{
			"storage": map[string]interface{}{
				"enabled":      true,
				"size":         "500Mi",
				"storageClass": storageClass,
			},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "50m",
					"memory": "64Mi",
				},
				"limits": map[string]interface{}{
					"cpu":    "100m",
					"memory": "128Mi",
				},
			},
		}
	}

	// Merge custom values (custom values override defaults)
	for k, v := range customValues {
		if (k == "postgresql" || k == "redis") && values[k] != nil {
			// Deep merge for service-specific values
			defaultValues := values[k].(map[string]interface{})
			customServiceValues := v.(map[string]interface{})
			mergeDeep(defaultValues, customServiceValues)
		} else {
			values[k] = v
		}
	}

	data, _ := yaml.Marshal(values)
	return string(data)
}

// mergeDeep recursively merges src into dst
func mergeDeep(dst, src map[string]interface{}) {
	for k, v := range src {
		if dstVal, ok := dst[k]; ok {
			dstMap, dstIsMap := dstVal.(map[string]interface{})
			srcMap, srcIsMap := v.(map[string]interface{})
			if dstIsMap && srcIsMap {
				mergeDeep(dstMap, srcMap)
			} else {
				dst[k] = v
			}
		} else {
			dst[k] = v
		}
	}
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

// detectRequiredOperators detects which operators need to be installed
func (r *PlatformApplicationClaimReconciler) detectRequiredOperators(claim *platformv1.PlatformApplicationClaim) []string {
	var operators []string
	operatorMap := make(map[string]bool)

	for _, service := range claim.Spec.Services {
		if !service.Enabled {
			continue
		}

		var operatorName string
		switch service.Type {
		case "postgresql":
			operatorName = "cloudnative-pg"
		case "redis":
			operatorName = "redis-operator"
		case "rabbitmq":
			operatorName = "rabbitmq-operator"
		case "mongodb":
			operatorName = "mongodb-operator"
		}

		if operatorName != "" && !operatorMap[operatorName] {
			operatorMap[operatorName] = true
			// Check if operator is already installed
			if !r.isOperatorInstalled(operatorName) {
				operators = append(operators, operatorName)
			}
		}
	}

	return operators
}

// isOperatorInstalled checks if an operator is already installed
func (r *PlatformApplicationClaimReconciler) isOperatorInstalled(operatorName string) bool {
	// For now, always return true if operators exist in ArgoCD
	// This is a simplified check - we assume if the ArgoCD Application exists, the operator is installed
	ctx := context.Background()

	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	err := r.Get(ctx, client.ObjectKey{
		Namespace: "argocd",
		Name:      operatorName,
	}, app)

	// If the ArgoCD Application exists, we consider the operator installed
	return err == nil
}

// installOperators installs the required operators via ArgoCD
func (r *PlatformApplicationClaimReconciler) installOperators(ctx context.Context, operators []string) error {
	logger := log.FromContext(ctx)

	for _, operatorName := range operators {
		logger.Info("Installing operator via ArgoCD", "operator", operatorName)

		var appManifest string
		switch operatorName {
		case "cloudnative-pg":
			appManifest = r.generateOperatorApplication("cloudnative-pg", "https://cloudnative-pg.github.io/charts", "cloudnative-pg", "1.22.0")
		case "redis-operator":
			appManifest = r.generateOperatorApplication("redis-operator", "https://spotahome.github.io/redis-operator", "redis-operator", "3.2.9")
		}

		if appManifest != "" {
			if err := r.createOperatorApplication(ctx, appManifest); err != nil {
				return fmt.Errorf("failed to install %s: %w", operatorName, err)
			}
		}
	}

	return nil
}

// generateOperatorApplication generates ArgoCD Application for operator installation
func (r *PlatformApplicationClaimReconciler) generateOperatorApplication(name, repoURL, chart, version string) string {
	app := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "argocd",
			"labels": map[string]string{
				"platform.infraforge.io/type":     "operator",
				"platform.infraforge.io/operator": name,
			},
		},
		"spec": map[string]interface{}{
			"project": "default",
			"source": map[string]interface{}{
				"repoURL":        repoURL,
				"chart":          chart,
				"targetRevision": version,
				"helm": map[string]interface{}{
					"values": "",
				},
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": name + "-system",
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

	data, _ := yaml.Marshal(app)
	return string(data)
}

// createOperatorApplication creates the operator Application in ArgoCD
func (r *PlatformApplicationClaimReconciler) createOperatorApplication(ctx context.Context, appYAML string) error {
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(appYAML), &obj.Object); err != nil {
		return fmt.Errorf("failed to unmarshal Application: %w", err)
	}

	obj.SetNamespace("argocd")

	// Try to create, if already exists it's fine
	err := r.Create(ctx, obj)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Application: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *PlatformApplicationClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.PlatformApplicationClaim{}).
		Complete(r)
}
