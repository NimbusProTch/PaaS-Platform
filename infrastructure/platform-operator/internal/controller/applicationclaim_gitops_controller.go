package controller

import (
	"context"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"github.com/infraforge/platform-operator/pkg/gitea"
	"github.com/infraforge/platform-operator/pkg/helm"
)

// ApplicationClaimGitOpsReconciler reconciles ApplicationClaim with GitOps
type ApplicationClaimGitOpsReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// GiteaClient for Git operations
	GiteaClient  *gitea.Client
	Organization string
	VoltranRepo  string
	Branch       string

	// OCIBaseURL base URL for OCI charts (e.g., "oci://ghcr.io/nimbusprotch")
	OCIBaseURL string
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
		return ctrl.Result{}, nil
	}

	// Update status to Provisioning
	claim.Status.Phase = "Provisioning"
	claim.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Generate ApplicationSet and values.yaml
	logger.Info("Generating ApplicationSet and values", "environment", claim.Spec.Environment)

	files := make(map[string]string)

	// Generate ApplicationSet
	appSetPath := fmt.Sprintf("appsets/%s/apps/%s-appset.yaml", claim.Spec.ClusterType, claim.Spec.Environment)
	appSetContent := r.generateApplicationSet(claim)
	files[appSetPath] = appSetContent

	// Generate directory structure for each application
	for _, app := range claim.Spec.Applications {
		// values.yaml
		valuesPath := fmt.Sprintf("environments/%s/%s/applications/%s/values.yaml", claim.Spec.ClusterType, claim.Spec.Environment, app.Name)
		valuesContent := r.generateValuesYAML(claim, app)
		files[valuesPath] = valuesContent

		// config.yaml (metadata)
		configPath := fmt.Sprintf("environments/%s/%s/applications/%s/config.yaml", claim.Spec.ClusterType, claim.Spec.Environment, app.Name)
		configContent := r.generateConfigYAML(app)
		files[configPath] = configContent
	}

	// Push to Gitea - use internal clone URL
	voltranURL := r.GiteaClient.ConstructCloneURL(r.Organization, r.VoltranRepo)
	commitMsg := fmt.Sprintf("Update %s environment applications by operator", claim.Spec.Environment)

	if err := r.GiteaClient.PushFiles(ctx, voltranURL, r.Branch, files, commitMsg,
		"Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push to Git")
		claim.Status.Phase = "Failed"
		claim.Status.LastUpdated = metav1.Now()
		r.Status().Update(ctx, claim)
		return ctrl.Result{}, err
	}

	// Update status to Ready
	claim.Status.Phase = "Ready"
	claim.Status.Ready = true
	claim.Status.ApplicationsReady = true
	claim.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
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
					"git": map[string]interface{}{
						"repoURL":  fmt.Sprintf("http://gitea.gitea.svc.cluster.local:3000/%s/%s", r.Organization, r.VoltranRepo),
						"revision": r.Branch,
						"directories": []map[string]interface{}{
							{
								"path": fmt.Sprintf("environments/%s/%s/applications/*", claim.Spec.ClusterType, claim.Spec.Environment),
							},
						},
						"files": []map[string]interface{}{
							{
								"path": fmt.Sprintf("environments/%s/%s/applications/*/config.yaml", claim.Spec.ClusterType, claim.Spec.Environment),
							},
						},
					},
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "{{path.basename}}-" + claim.Spec.Environment,
					"labels": map[string]string{
						"platform.infraforge.io/app": "{{path.basename}}",
						"platform.infraforge.io/env": claim.Spec.Environment,
					},
				},
				"spec": map[string]interface{}{
					"project": "default",
					"sources": []map[string]interface{}{
						{
							// OCI chart source
							"repoURL":        fmt.Sprintf("%s/{{config.chart}}", r.OCIBaseURL),
							"chart":          "{{config.chart}}",
							"targetRevision": "{{config.version}}",
							"helm": map[string]interface{}{
								"valueFiles": []string{
									"$values/environments/" + claim.Spec.ClusterType + "/" + claim.Spec.Environment + "/applications/{{path.basename}}/values.yaml",
								},
							},
						},
						{
							// Values repository source
							"repoURL":        fmt.Sprintf("http://gitea-http.gitea.svc.cluster.local:3000/%s/%s", r.Organization, r.VoltranRepo),
							"targetRevision": r.Branch,
							"ref":            "values",
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

// generateConfigYAML generates config.yaml with chart metadata
func (r *ApplicationClaimGitOpsReconciler) generateConfigYAML(app platformv1.ApplicationSpec) string {
	config := map[string]interface{}{
		"chart":   app.Chart.Name,
		"version": app.Chart.Version,
	}

	if app.Chart.Version == "" {
		config["version"] = "1.0.0"
	}

	yamlBytes, _ := yaml.Marshal(config)
	return string(yamlBytes)
}

// generateValuesYAML generates Helm values.yaml for an application using smart merging
// 1. Pull chart from OCI registry
// 2. Read base values.yaml
// 3. If production environment, merge values-production.yaml
// 4. Apply CRD custom overrides
func (r *ApplicationClaimGitOpsReconciler) generateValuesYAML(claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) string {
	logger := ctrl.Log.WithName("generateValues")

	// Initialize Helm client
	helmClient := helm.NewClient()

	// Determine chart name and version
	chartName := app.Chart.Name
	if chartName == "" {
		chartName = "microservice" // default chart
	}
	chartVersion := app.Chart.Version
	if chartVersion == "" {
		chartVersion = "1.0.0" // default version
	}

	// Build OCI chart URL
	chartURL := fmt.Sprintf("%s/%s", r.OCIBaseURL, chartName)

	// Step 1: Pull chart from OCI registry
	chartPath, err := helmClient.PullOCIChart(context.Background(), chartURL, chartVersion)
	if err != nil {
		logger.Error(err, "failed to pull OCI chart, using CRD values only", "chart", chartURL, "version", chartVersion)
		return r.generateValuesFromCRD(claim, app)
	}

	// Step 2: Read base values.yaml
	baseValuesPath := filepath.Join(chartPath, "values.yaml")
	baseValues, err := helmClient.ReadValuesFile(baseValuesPath)
	if err != nil {
		logger.Error(err, "failed to read base values.yaml, using CRD values only", "path", baseValuesPath)
		return r.generateValuesFromCRD(claim, app)
	}

	// Step 3: Determine if production environment
	isProd := claim.Spec.Environment == "prod" || claim.Spec.ClusterType == "prod"

	finalValues := baseValues

	// Step 4: Merge production values if applicable
	if isProd {
		prodValuesPath := filepath.Join(chartPath, "values-production.yaml")
		prodValues, err := helmClient.ReadValuesFile(prodValuesPath)
		if err == nil {
			logger.Info("Merging production values", "chart", chartName)
			finalValues = helmClient.MergeValues(finalValues, prodValues)
		} else {
			logger.Info("No production values found, using base values only", "path", prodValuesPath)
		}
	}

	// Step 5: Apply CRD custom overrides
	crdOverrides := r.buildCRDOverrides(app)
	finalValues = helmClient.MergeValues(finalValues, crdOverrides)

	// Marshal to YAML
	data, _ := yaml.Marshal(finalValues)
	return string(data)
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

// SetupWithManager sets up the controller with the Manager
func (r *ApplicationClaimGitOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.ApplicationClaim{}).
		Complete(r)
}
