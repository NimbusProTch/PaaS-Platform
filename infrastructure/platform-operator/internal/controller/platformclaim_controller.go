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

// PlatformClaimReconciler reconciles a PlatformClaim object
type PlatformClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// GiteaClient for Git operations
	GiteaClient  *gitea.Client
	Organization string
	VoltranRepo  string
	Branch       string
}

//+kubebuilder:rbac:groups=platform.infraforge.io,resources=platformclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=platformclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=platformclaims/finalizers,verbs=update

// Reconcile handles PlatformClaim reconciliation
func (r *PlatformClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling PlatformClaim", "name", req.Name, "namespace", req.Namespace)

	// Fetch the PlatformClaim
	claim := &platformv1.PlatformClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch PlatformClaim")
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

	// Generate ApplicationSet and values.yaml for platform services
	logger.Info("Generating platform ApplicationSet and values", "environment", claim.Spec.Environment)

	files := make(map[string]string)

	// Generate ApplicationSet for platform services
	appSetPath := fmt.Sprintf("appsets/%s/platform/%s-platform-appset.yaml", claim.Spec.ClusterType, claim.Spec.Environment)
	appSetContent := r.generatePlatformApplicationSet(claim)
	files[appSetPath] = appSetContent

	// Generate values.yaml for each service
	for _, service := range claim.Spec.Services {
		valuesPath := fmt.Sprintf("environments/%s/%s/platform/%s/values.yaml", claim.Spec.ClusterType, claim.Spec.Environment, service.Name)
		valuesContent := r.generatePlatformValuesYAML(claim, service)
		files[valuesPath] = valuesContent
	}

	// Push to Gitea - use internal clone URL
	voltranURL := r.GiteaClient.ConstructCloneURL(r.Organization, r.VoltranRepo)
	commitMsg := fmt.Sprintf("Update %s environment platform services by operator", claim.Spec.Environment)

	if err := r.GiteaClient.PushFiles(ctx, voltranURL, r.Branch, files, commitMsg,
		"Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push to Git")
		claim.Status.Phase = "Failed"
		claim.Status.Message = fmt.Sprintf("Failed to push to Git: %v", err)
		claim.Status.LastUpdated = metav1.Now()
		r.Status().Update(ctx, claim)
		return ctrl.Result{}, err
	}

	// Update status to Ready
	claim.Status.Phase = "Ready"
	claim.Status.Ready = true
	claim.Status.ServicesReady = true
	claim.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("PlatformClaim reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// generatePlatformApplicationSet generates ArgoCD ApplicationSet for platform services
func (r *PlatformClaimReconciler) generatePlatformApplicationSet(claim *platformv1.PlatformClaim) string {
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
						"elements": r.generatePlatformElements(claim),
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
						"repoURL":        fmt.Sprintf("http://gitea.gitea.svc.cluster.local:3000/%s/charts", r.Organization),
						"path":           "{{chart}}",
						"targetRevision": r.Branch,
						"helm": map[string]interface{}{
							"valueFiles": []string{
								fmt.Sprintf("../../voltran/environments/%s/%s/platform/{{service}}/values.yaml", claim.Spec.ClusterType, claim.Spec.Environment),
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

// generatePlatformElements generates list elements for platform ApplicationSet
func (r *PlatformClaimReconciler) generatePlatformElements(claim *platformv1.PlatformClaim) []map[string]string {
	elements := []map[string]string{}

	for _, service := range claim.Spec.Services {
		chartName := service.Chart.Name
		if chartName == "" {
			chartName = service.Type // fallback to type
		}

		elements = append(elements, map[string]string{
			"service":     service.Name,
			"chart":       chartName,
			"environment": claim.Spec.Environment,
		})
	}

	return elements
}

// generatePlatformValuesYAML generates Helm values.yaml for a platform service
func (r *PlatformClaimReconciler) generatePlatformValuesYAML(claim *platformv1.PlatformClaim, service platformv1.PlatformServiceSpec) string {
	values := map[string]interface{}{}

	// Environment-specific configuration based on size
	switch service.Size {
	case "large":
		values["resources"] = map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "2000m",
				"memory": "4Gi",
			},
			"limits": map[string]interface{}{
				"cpu":    "4000m",
				"memory": "8Gi",
			},
		}
	case "small":
		values["resources"] = map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "100m",
				"memory": "256Mi",
			},
			"limits": map[string]interface{}{
				"cpu":    "200m",
				"memory": "512Mi",
			},
		}
	default: // medium
		values["resources"] = map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "500m",
				"memory": "1Gi",
			},
			"limits": map[string]interface{}{
				"cpu":    "1000m",
				"memory": "2Gi",
			},
		}
	}

	// High Availability
	if service.HighAvailability {
		values["replicaCount"] = 3
		values["podDisruptionBudget"] = map[string]interface{}{
			"enabled":        true,
			"minAvailable":   2,
			"maxUnavailable": 1,
		}
	}

	// Backup configuration
	if service.Backup != nil && service.Backup.Enabled {
		values["backup"] = map[string]interface{}{
			"enabled":      true,
			"schedule":     service.Backup.Schedule,
			"retention":    service.Backup.Retention,
			"storageClass": service.Backup.StorageClass,
		}
	}

	// Monitoring
	if service.Monitoring {
		values["metrics"] = map[string]interface{}{
			"enabled": true,
			"serviceMonitor": map[string]interface{}{
				"enabled": true,
			},
		}
	}

	// Service version
	if service.Version != "" {
		values["image"] = map[string]interface{}{
			"tag": service.Version,
		}
	}

	// Custom values from spec (merge with generated values)
	if service.Values.Raw != nil {
		var customValues map[string]interface{}
		if err := yaml.Unmarshal(service.Values.Raw, &customValues); err == nil {
			for k, v := range customValues {
				values[k] = v
			}
		}
	}

	data, _ := yaml.Marshal(values)
	return string(data)
}

// SetupWithManager sets up the controller with the Manager
func (r *PlatformClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.PlatformClaim{}).
		Complete(r)
}
