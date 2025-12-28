package controller

import (
	"context"
	"fmt"
	"time"

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

	// Generate values.yaml for each service
	for _, service := range claim.Spec.Services {
		// Skip disabled services
		if !service.Enabled {
			logger.Info("Skipping disabled platform service", "name", service.Name)
			continue
		}

		valuesPath := fmt.Sprintf("environments/%s/%s/platform/%s/values.yaml", claim.Spec.ClusterType, claim.Spec.Environment, service.Name)
		valuesContent := r.generatePlatformValuesYAML(claim, service, giteaClient)
		files[valuesPath] = valuesContent
	}

	// Push to Gitea - use internal clone URL
	voltranURL := giteaClient.ConstructCloneURL(claim.Spec.Organization, r.VoltranRepo)
	commitMsg := fmt.Sprintf("Update %s environment platform services by operator", claim.Spec.Environment)

	if err := giteaClient.PushFiles(ctx, voltranURL, r.Branch, files, commitMsg,
		"Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push to Git")
		// Don't update status on git errors, just retry
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

// generatePlatformApplicationSet generates ArgoCD ApplicationSet for platform services
func (r *PlatformApplicationClaimReconciler) generatePlatformApplicationSet(claim *platformv1.PlatformApplicationClaim, giteaClient *gitea.Client) string {
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
						"elements": r.generatePlatformElements(claim, giteaClient),
					},
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "{{service}}-{{environment}}",
					"labels": map[string]string{
						"platform.infraforge.io/service": "{{service}}",
						"platform.infraforge.io/env":     "{{environment}}",
						"platform.infraforge.io/type":    "platform",
					},
				},
				"spec": map[string]interface{}{
					"project": "default",
					"source": map[string]interface{}{
						"repoURL":        "oci://ghcr.io/nimbusprotch",
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

// generatePlatformElements generates list elements for platform ApplicationSet
func (r *PlatformApplicationClaimReconciler) generatePlatformElements(claim *platformv1.PlatformApplicationClaim, giteaClient *gitea.Client) []map[string]string {
	elements := []map[string]string{}

	for _, service := range claim.Spec.Services {
		// Skip disabled services
		if !service.Enabled {
			continue
		}

		chartName := service.Chart.Name
		if chartName == "" {
			chartName = service.Type // fallback to type
		}

		valuesYAML := r.generatePlatformValuesYAML(claim, service, giteaClient)

		elements = append(elements, map[string]string{
			"service":     service.Name,
			"chart":       chartName,
			"environment": claim.Spec.Environment,
			"version":     "1.0.0", // Chart version
			"values":      valuesYAML,
		})
	}

	return elements
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

// SetupWithManager sets up the controller with the Manager
func (r *PlatformApplicationClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.PlatformApplicationClaim{}).
		Complete(r)
}
