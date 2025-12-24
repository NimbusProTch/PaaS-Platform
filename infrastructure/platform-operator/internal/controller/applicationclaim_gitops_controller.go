package controller

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
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

	// GiteaClient for Git operations
	GiteaClient  *gitea.Client
	Organization string
	VoltranRepo  string
	Branch       string
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
	appSetPath := fmt.Sprintf("appsets/%s-applications.yaml", claim.Spec.Environment)
	appSetContent := r.generateApplicationSet(claim)
	files[appSetPath] = appSetContent

	// Generate directory structure for each application
	for _, app := range claim.Spec.Applications {
		// values.yaml
		valuesPath := fmt.Sprintf("environments/%s/%s/values.yaml", claim.Spec.Environment, app.Name)
		valuesContent := r.generateValuesYAML(claim, app)
		files[valuesPath] = valuesContent

		// config.yaml (metadata)
		configPath := fmt.Sprintf("environments/%s/%s/config.yaml", claim.Spec.Environment, app.Name)
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
								"path": fmt.Sprintf("environments/%s/*", claim.Spec.Environment),
							},
						},
						"files": []map[string]interface{}{
							{
								"path": fmt.Sprintf("environments/%s/*/config.yaml", claim.Spec.Environment),
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
					"source": map[string]interface{}{
						"repoURL":        fmt.Sprintf("http://gitea.gitea.svc.cluster.local:3000/%s/charts", r.Organization),
						"path":           "{{config.chart}}",
						"targetRevision": r.Branch,
						"helm": map[string]interface{}{
							"valueFiles": []string{
								fmt.Sprintf("../../%s/{{path}}/values.yaml", r.VoltranRepo),
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

// generateValuesYAML generates Helm values.yaml for an application
func (r *ApplicationClaimGitOpsReconciler) generateValuesYAML(claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) string {
	values := map[string]interface{}{
		"image": map[string]interface{}{
			"repository": app.Image.Repository,
			"tag":        app.Image.Tag,
		},
		"replicaCount": app.Replicas,
	}

	if app.Replicas == 0 {
		values["replicaCount"] = 1
	}

	// Add resources if specified
	if app.Resources.Requests.CPU != "" || app.Resources.Requests.Memory != "" {
		resources := map[string]interface{}{}
		if app.Resources.Requests.CPU != "" || app.Resources.Requests.Memory != "" {
			resources["requests"] = map[string]interface{}{
				"cpu":    app.Resources.Requests.CPU,
				"memory": app.Resources.Requests.Memory,
			}
		}
		if app.Resources.Limits.CPU != "" || app.Resources.Limits.Memory != "" {
			resources["limits"] = map[string]interface{}{
				"cpu":    app.Resources.Limits.CPU,
				"memory": app.Resources.Limits.Memory,
			}
		}
		values["resources"] = resources
	}

	// Add ingress if specified
	if app.Ingress != nil && app.Ingress.Enabled {
		values["ingress"] = map[string]interface{}{
			"enabled": true,
			"host":    app.Ingress.Host,
			"path":    app.Ingress.Path,
			"tls":     app.Ingress.TLS,
		}
		if len(app.Ingress.Annotations) > 0 {
			values["ingress"].(map[string]interface{})["annotations"] = app.Ingress.Annotations
		}
	}

	// Add environment variables
	if len(app.Env) > 0 {
		envVars := []map[string]interface{}{}
		for _, env := range app.Env {
			envVar := map[string]interface{}{
				"name":  env.Name,
				"value": env.Value,
			}
			envVars = append(envVars, envVar)
		}
		values["env"] = envVars
	}

	data, _ := yaml.Marshal(values)
	return string(data)
}

// SetupWithManager sets up the controller with the Manager
func (r *ApplicationClaimGitOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.ApplicationClaim{}).
		Complete(r)
}
