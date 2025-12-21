package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"github.com/infraforge/platform-operator/internal/clients"
	"github.com/infraforge/platform-operator/pkg/argocd"
	"github.com/infraforge/platform-operator/pkg/helm"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ApplicationClaimReconciler reconciles a ApplicationClaim object
type ApplicationClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// External clients
	GitHubClient *clients.GitHubClient
	HelmClient   *helm.Client
	ArgoCDClient *argocd.Client
}

//+kubebuilder:rbac:groups=platform.infraforge.io,resources=applicationclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=applicationclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=applicationclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=argoproj.io,resources=appprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=argoproj.io,resources=appprojects,verbs=get;list;watch;create;update;patch;delete

func (r *ApplicationClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ApplicationClaim
	claim := &platformv1.ApplicationClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, cleanup handled by finalizers
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch ApplicationClaim")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if claim.Status.Phase == "" {
		claim.Status.Phase = "Pending"
		claim.Status.LastUpdated = metav1.Now()
		// Retry the status update with fresh object
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Fetch the latest version
			latest := &platformv1.ApplicationClaim{}
			if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
				return err
			}
			// Update status
			latest.Status.Phase = "Pending"
			latest.Status.LastUpdated = metav1.Now()
			return r.Status().Update(ctx, latest)
		})
		if retryErr != nil {
			logger.Error(retryErr, "failed to update status after retries")
			return ctrl.Result{}, retryErr
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Handle finalizer
	finalizerName := "platform.infraforge.io/finalizer"
	if claim.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !containsString(claim.ObjectMeta.Finalizers, finalizerName) {
			claim.ObjectMeta.Finalizers = append(claim.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, claim); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// Being deleted
		if containsString(claim.ObjectMeta.Finalizers, finalizerName) {
			// Cleanup
			if err := r.deleteExternalResources(ctx, claim); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			claim.ObjectMeta.Finalizers = removeString(claim.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, claim); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Start provisioning with retry
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version
		latest := &platformv1.ApplicationClaim{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			return err
		}

		latest.Status.Phase = "Provisioning"
		latest.Status.LastUpdated = metav1.Now()
		// Clear status arrays to rebuild from scratch
		latest.Status.Applications = []platformv1.ApplicationStatus{}
		latest.Status.Components = []platformv1.ComponentStatus{}

		return r.Status().Update(ctx, latest)
	})

	if retryErr != nil {
		logger.Error(retryErr, "failed to update status after retries")
		return ctrl.Result{}, retryErr
	}

	// Refresh claim object after status update
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Use ArgoCD-based deployment instead of direct deployment
	if err := r.reconcileWithArgoCD(ctx, claim); err != nil {
		logger.Error(err, "failed to reconcile with ArgoCD")
		claim.Status.Phase = "Failed"
		r.Status().Update(ctx, claim)
		return ctrl.Result{}, err
	}

	// Update final status with retry
	finalErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version
		latest := &platformv1.ApplicationClaim{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		latest.Status.Phase = "Ready"
		latest.Status.LastUpdated = metav1.Now()
		// Copy the status from our working copy
		latest.Status.Applications = claim.Status.Applications
		latest.Status.Components = claim.Status.Components

		return r.Status().Update(ctx, latest)
	})

	if finalErr != nil {
		logger.Error(finalErr, "failed to update final status")
		return ctrl.Result{}, finalErr
	}

	logger.Info("ApplicationClaim successfully reconciled", "name", claim.Name)
	// Don't requeue - let events trigger reconciliation
	return ctrl.Result{}, nil
}

func (r *ApplicationClaimReconciler) reconcileComponent(ctx context.Context, claim *platformv1.ApplicationClaim, component platformv1.ComponentSpec) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling component", "type", component.Type, "name", component.Name)

	// Get component configuration based on environment
	componentConfig := r.getComponentConfig(claim.Spec.Environment, component)

	// Deploy using Helm
	if r.HelmClient != nil {
		releaseName := fmt.Sprintf("%s-%s", claim.Name, component.Name)
		namespace := claim.Spec.Namespace
		if namespace == "" {
			namespace = claim.Namespace
		}

		if err := r.HelmClient.InstallOrUpgrade(ctx, helm.Release{
			Name:      releaseName,
			Namespace: namespace,
			Chart:     r.getChartForComponent(component.Type),
			Values:    componentConfig,
		}); err != nil {
			return fmt.Errorf("failed to install helm chart: %w", err)
		}
	}

	// Add component status (arrays are cleared at the beginning of reconciliation)
	claim.Status.Components = append(claim.Status.Components, platformv1.ComponentStatus{
		Name:       component.Name,
		Type:       component.Type,
		Ready:      true,
		SecretName: fmt.Sprintf("%s-%s-secret", claim.Name, component.Name),
	})

	return nil
}

func (r *ApplicationClaimReconciler) reconcileApplication(ctx context.Context, claim *platformv1.ApplicationClaim, app platformv1.ApplicationSpec) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling application", "name", app.Name, "version", app.Version)

	// Deploy application directly using Kubernetes manifests
	namespace := claim.Spec.Namespace
	if namespace == "" {
		namespace = claim.Namespace
	}

	// Create namespace if not exists
	if err := r.ensureNamespace(ctx, namespace); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Check if this is a GitHub release deployment
	if app.Repository != "" && app.Version != "" {
		logger.Info("Deploying from GitHub release", "repo", app.Repository, "version", app.Version)
		if err := r.deployFromGitHub(ctx, namespace, app); err != nil {
			logger.Error(err, "Failed to deploy from GitHub, falling back to direct deployment")
			// Fall back to direct deployment
			if err := r.deployBackendService(ctx, namespace, app); err != nil {
				return fmt.Errorf("failed to deploy application: %w", err)
			}
		}
	} else {
		// For now, deploy our test backend-service directly
		if err := r.deployBackendService(ctx, namespace, app); err != nil {
			return fmt.Errorf("failed to deploy application: %w", err)
		}
	}

	// Add application status (arrays are cleared at the beginning of reconciliation)
	claim.Status.Applications = append(claim.Status.Applications, platformv1.ApplicationStatus{
		Name:     app.Name,
		Ready:    true,
		Version:  app.Version,
		Replicas: app.Replicas,
	})

	return nil
}

func (r *ApplicationClaimReconciler) createArgoCDAppOfAppsApplication(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)
	logger.Info("Creating ArgoCD app-of-apps application", "claim", claim.Name)

	if r.ArgoCDClient == nil {
		logger.Info("ArgoCD client not configured, skipping")
		return nil
	}

	// Generate app-of-apps configuration
	appConfig := r.generateAppOfAppsConfig(claim)

	// Create ArgoCD Application
	if err := r.ArgoCDClient.CreateApplication(ctx, appConfig); err != nil {
		return fmt.Errorf("failed to create ArgoCD application: %w", err)
	}

	return nil
}

func (r *ApplicationClaimReconciler) generateAppOfAppsConfig(claim *platformv1.ApplicationClaim) argocd.ApplicationSpec {
	namespace := claim.Spec.Namespace
	if namespace == "" {
		namespace = claim.Namespace
	}

	return argocd.ApplicationSpec{
		Name:      fmt.Sprintf("%s-apps", claim.Name),
		Namespace: "argocd",
		Project:   "default",
		Source: argocd.ApplicationSource{
			RepoURL:        "https://github.com/infraforge/platform-configs",
			Path:           fmt.Sprintf("claims/%s", claim.Name),
			TargetRevision: "main",
		},
		Destination: argocd.ApplicationDestination{
			Server:    "https://kubernetes.default.svc",
			Namespace: namespace,
		},
		SyncPolicy: &argocd.SyncPolicy{
			Automated: &argocd.SyncPolicyAutomated{
				Prune:    true,
				SelfHeal: true,
			},
		},
	}
}

func (r *ApplicationClaimReconciler) storeManifests(ctx context.Context, claim *platformv1.ApplicationClaim, appName string, manifests map[string]string) error {
	namespace := claim.Spec.Namespace
	if namespace == "" {
		namespace = claim.Namespace
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-manifests", claim.Name, appName),
			Namespace: namespace,
		},
		Data: manifests,
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(claim, cm, r.Scheme); err != nil {
		return err
	}

	// Create or update ConfigMap
	existingCM := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existingCM); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, cm)
		}
		return err
	}

	existingCM.Data = cm.Data
	return r.Update(ctx, existingCM)
}

func (r *ApplicationClaimReconciler) ensureNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := r.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, ns)
		}
		return err
	}

	return nil
}

func (r *ApplicationClaimReconciler) getComponentConfig(environment string, component platformv1.ComponentSpec) map[string]interface{} {
	config := make(map[string]interface{})

	// Base configuration
	config["fullnameOverride"] = component.Name

	// Environment-specific configuration
	switch environment {
	case "prod":
		config["replicaCount"] = 3
		config["resources"] = map[string]interface{}{
			"requests": map[string]string{
				"memory": "1Gi",
				"cpu":    "500m",
			},
			"limits": map[string]string{
				"memory": "2Gi",
				"cpu":    "1000m",
			},
		}
		if component.Type == "postgresql" {
			config["persistence"] = map[string]interface{}{
				"size": "100Gi",
			}
			config["backup"] = map[string]interface{}{
				"enabled": true,
			}
		}
	case "staging":
		config["replicaCount"] = 2
		config["resources"] = map[string]interface{}{
			"requests": map[string]string{
				"memory": "512Mi",
				"cpu":    "250m",
			},
			"limits": map[string]string{
				"memory": "1Gi",
				"cpu":    "500m",
			},
		}
		if component.Type == "postgresql" {
			config["persistence"] = map[string]interface{}{
				"size": "20Gi",
			}
		}
	default: // dev
		config["replicaCount"] = 1
		config["resources"] = map[string]interface{}{
			"requests": map[string]string{
				"memory": "256Mi",
				"cpu":    "100m",
			},
			"limits": map[string]string{
				"memory": "512Mi",
				"cpu":    "250m",
			},
		}
		if component.Type == "postgresql" {
			config["persistence"] = map[string]interface{}{
				"size": "10Gi",
			}
		}
	}

	// Apply custom config
	for k, v := range component.Config {
		config[k] = v
	}

	// Size-based overrides
	switch component.Size {
	case "large":
		if res, ok := config["resources"].(map[string]interface{}); ok {
			res["requests"] = map[string]string{
				"memory": "2Gi",
				"cpu":    "1000m",
			}
			res["limits"] = map[string]string{
				"memory": "4Gi",
				"cpu":    "2000m",
			}
		}
	case "small":
		if res, ok := config["resources"].(map[string]interface{}); ok {
			res["requests"] = map[string]string{
				"memory": "128Mi",
				"cpu":    "50m",
			}
			res["limits"] = map[string]string{
				"memory": "256Mi",
				"cpu":    "100m",
			}
		}
	}

	return config
}

func (r *ApplicationClaimReconciler) getChartForComponent(componentType string) string {
	charts := map[string]string{
		"postgresql":    "bitnami/postgresql",
		"redis":         "bitnami/redis",
		"rabbitmq":      "bitnami/rabbitmq",
		"mongodb":       "bitnami/mongodb",
		"mysql":         "bitnami/mysql",
		"kafka":         "bitnami/kafka",
		"elasticsearch": "elastic/elasticsearch",
	}

	if chart, ok := charts[componentType]; ok {
		return chart
	}
	return "bitnami/" + componentType
}

func (r *ApplicationClaimReconciler) deleteExternalResources(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)
	logger.Info("Deleting external resources", "claim", claim.Name)

	// Delete ArgoCD applications
	if r.ArgoCDClient != nil {
		appName := fmt.Sprintf("%s-apps", claim.Name)
		if err := r.ArgoCDClient.DeleteApplication(ctx, appName); err != nil {
			logger.Error(err, "failed to delete ArgoCD application")
		}
	}

	// Delete Helm releases
	if r.HelmClient != nil {
		namespace := claim.Spec.Namespace
		if namespace == "" {
			namespace = claim.Namespace
		}

		for _, component := range claim.Spec.Components {
			releaseName := fmt.Sprintf("%s-%s", claim.Name, component.Name)
			if err := r.HelmClient.Uninstall(ctx, releaseName, namespace); err != nil {
				logger.Error(err, "failed to uninstall helm release", "release", releaseName)
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.ApplicationClaim{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

// Helper functions
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

func (r *ApplicationClaimReconciler) deployFromGitHub(ctx context.Context, namespace string, app platformv1.ApplicationSpec) error {
	logger := log.FromContext(ctx)
	logger.Info("Deploying from GitHub release", "repo", app.Repository, "version", app.Version)

	// Parse repository format (e.g., "github.com/owner/repo" or "owner/repo")
	parts := strings.Split(strings.TrimPrefix(app.Repository, "github.com/"), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid repository format: %s", app.Repository)
	}
	owner := parts[0]
	repo := parts[1]

	// Get manifests from GitHub release
	manifestPath, err := r.GitHubClient.GetManifests(ctx, owner, repo, app.Version)
	if err != nil {
		return fmt.Errorf("failed to get manifests from GitHub: %w", err)
	}

	// Apply manifests to the cluster
	// For now, we'll just log this - in production you would use a kubectl apply
	logger.Info("Would apply manifests from", "path", manifestPath)

	// TODO: Implement actual manifest application
	// This would involve reading YAML files from manifestPath and applying them
	// You could use client-go's dynamic client or exec kubectl apply

	return fmt.Errorf("GitHub deployment not yet fully implemented")
}

func (r *ApplicationClaimReconciler) deployBackendService(ctx context.Context, namespace string, app platformv1.ApplicationSpec) error {
	logger := log.FromContext(ctx)
	logger.Info("Deploying backend service", "namespace", namespace, "app", app.Name)

	// Deploy the backend service deployment and service
	// For now, we'll apply our test app directly

	// Create Deployment
	deployment := r.createDeployment(namespace, app)
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: namespace}, existingDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deployment); err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
		} else {
			return err
		}
	}

	// Create Service
	service := r.createService(namespace, app)
	svc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: namespace}, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, service); err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}
		} else {
			return err
		}
	}

	return nil
}

func (r *ApplicationClaimReconciler) createDeployment(namespace string, app platformv1.ApplicationSpec) *appsv1.Deployment {
	replicas := app.Replicas
	if replicas == 0 {
		replicas = 1
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     app.Name,
				"version": app.Version,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": app.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     app.Name,
						"version": app.Version,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      app.Name,
							Image:     fmt.Sprintf("%s:%s", app.Name, app.Version),
							Ports:     r.getContainerPorts(app),
							Env:       r.getEnvVars(app),
							Resources: r.getResourceRequirements(app),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}
}

func (r *ApplicationClaimReconciler) createService(namespace string, app platformv1.ApplicationSpec) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": app.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": app.Name,
			},
			Ports: r.getServicePorts(app),
		},
	}
}

func (r *ApplicationClaimReconciler) getContainerPorts(app platformv1.ApplicationSpec) []corev1.ContainerPort {
	if len(app.Ports) == 0 {
		return []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	}

	var ports []corev1.ContainerPort
	for _, port := range app.Ports {
		ports = append(ports, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.Port,
			Protocol:      corev1.Protocol(port.Protocol),
		})
	}
	return ports
}

func (r *ApplicationClaimReconciler) getServicePorts(app platformv1.ApplicationSpec) []corev1.ServicePort {
	if len(app.Ports) == 0 {
		return []corev1.ServicePort{
			{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	}

	var ports []corev1.ServicePort
	for _, port := range app.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: intstr.FromInt(int(port.Port)),
			Protocol:   corev1.Protocol(port.Protocol),
		})
	}
	return ports
}

func (r *ApplicationClaimReconciler) getEnvVars(app platformv1.ApplicationSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "APP_VERSION",
			Value: app.Version,
		},
		{
			Name:  "ENVIRONMENT",
			Value: "development",
		},
	}

	for _, env := range app.Env {
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name: env.Name,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: env.ValueFrom.SecretKeyRef.Name,
						},
						Key:      env.ValueFrom.SecretKeyRef.Key,
						Optional: func(b bool) *bool { return &b }(true),
					},
				},
			})
		} else {
			envVars = append(envVars, corev1.EnvVar{
				Name:  env.Name,
				Value: env.Value,
			})
		}
	}

	return envVars
}

func (r *ApplicationClaimReconciler) getResourceRequirements(app platformv1.ApplicationSpec) corev1.ResourceRequirements {
	requirements := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	if app.Resources.Requests.CPU != "" {
		requirements.Requests[corev1.ResourceCPU] = resource.MustParse(app.Resources.Requests.CPU)
	}
	if app.Resources.Requests.Memory != "" {
		requirements.Requests[corev1.ResourceMemory] = resource.MustParse(app.Resources.Requests.Memory)
	}
	if app.Resources.Limits.CPU != "" {
		requirements.Limits[corev1.ResourceCPU] = resource.MustParse(app.Resources.Limits.CPU)
	}
	if app.Resources.Limits.Memory != "" {
		requirements.Limits[corev1.ResourceMemory] = resource.MustParse(app.Resources.Limits.Memory)
	}

	return requirements
}
