package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"github.com/infraforge/platform-operator/pkg/gitea"
)

// BootstrapReconciler reconciles a BootstrapClaim object
type BootstrapReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// GiteaClient for Git operations
	GiteaClient *gitea.Client

	// ChartsPath embedded charts directory path (contains both microservice and platform templates)
	ChartsPath string
}

//+kubebuilder:rbac:groups=platform.infraforge.io,resources=bootstrapclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=bootstrapclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.infraforge.io,resources=bootstrapclaims/finalizers,verbs=update

// Reconcile handles BootstrapClaim reconciliation
func (r *BootstrapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling BootstrapClaim", "name", req.Name)

	// Fetch the BootstrapClaim
	claim := &platformv1.BootstrapClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch BootstrapClaim")
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
	if claim.Status.Ready {
		logger.Info("BootstrapClaim already ready", "name", claim.Name)
		return ctrl.Result{}, nil
	}

	// Update status to Bootstrapping
	claim.Status.Phase = "Bootstrapping"
	claim.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Step 1: Create organization
	logger.Info("Creating Gitea organization", "org", claim.Spec.Organization)
	if err := r.GiteaClient.CreateOrganization(ctx, claim.Spec.Organization, "Platform organization"); err != nil {
		logger.Error(err, "failed to create organization")
		r.updateStatusFailed(ctx, claim, "Failed to create organization: "+err.Error())
		return ctrl.Result{}, err
	}

	// Step 2: Create repositories
	logger.Info("Creating repositories")
	repoURLs := make(map[string]string)

	chartsRepo := claim.Spec.Repositories.Charts
	if chartsRepo == "" {
		chartsRepo = "charts"
	}
	voltranRepo := claim.Spec.Repositories.Voltran
	if voltranRepo == "" {
		voltranRepo = "voltran"
	}

	branch := claim.Spec.GitOps.Branch
	if branch == "" {
		branch = "main"
	}

	repos := []string{chartsRepo, voltranRepo}
	for _, repoName := range repos {
		_, err := r.GiteaClient.CreateRepository(ctx, claim.Spec.Organization, gitea.CreateRepoOptions{
			Name:          repoName,
			Description:   fmt.Sprintf("Platform %s repository", repoName),
			Private:       false,
			AutoInit:      true,
			DefaultBranch: branch,
		})
		if err != nil {
			logger.Error(err, "failed to create repository", "repo", repoName)
			r.updateStatusFailed(ctx, claim, fmt.Sprintf("Failed to create repository %s: %v", repoName, err))
			return ctrl.Result{}, err
		}
		// Use internal cluster URL instead of API's external clone_url
		cloneURL := r.GiteaClient.ConstructCloneURL(claim.Spec.Organization, repoName)
		repoURLs[repoName] = cloneURL
		logger.Info("Repository created", "name", repoName, "url", cloneURL)
	}

	claim.Status.RepositoriesCreated = true
	claim.Status.RepositoryURLs = repoURLs
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Step 3: Upload charts to charts repository (contains both app and platform templates)
	logger.Info("Uploading charts (microservice & platform templates)", "repo", chartsRepo)

	var chartFiles map[string]string
	var err error

	// Check if external charts repository is specified
	if claim.Spec.ChartsRepository != nil {
		repoType := claim.Spec.ChartsRepository.Type
		if repoType == "" {
			repoType = "git" // Default to git for backwards compatibility
		}

		logger.Info("Loading charts from external repository", "url", claim.Spec.ChartsRepository.URL, "type", repoType)

		if repoType == "oci" {
			// Pull from OCI registry
			version := claim.Spec.ChartsRepository.Version
			if version == "" {
				version = "latest"
			}

			logger.Info("Pulling chart from OCI registry", "version", version)
			chartFiles, err = r.GiteaClient.PullOCIChartAndExtract(ctx, claim.Spec.ChartsRepository.URL, version)
			if err != nil {
				logger.Error(err, "failed to pull charts from OCI registry")
				r.updateStatusFailed(ctx, claim, "Failed to pull OCI chart: "+err.Error())
				return ctrl.Result{}, err
			}
		} else {
			// Clone from Git repository
			chartsBranch := claim.Spec.ChartsRepository.Branch
			if chartsBranch == "" {
				chartsBranch = "main"
			}
			chartsPath := claim.Spec.ChartsRepository.Path

			logger.Info("Cloning charts from Git repository", "branch", chartsBranch, "path", chartsPath)
			chartFiles, err = r.GiteaClient.CloneAndExtractFiles(ctx, claim.Spec.ChartsRepository.URL, chartsBranch, chartsPath)
			if err != nil {
				logger.Error(err, "failed to clone charts from Git repository")
				r.updateStatusFailed(ctx, claim, "Failed to clone charts: "+err.Error())
				return ctrl.Result{}, err
			}
		}
	} else {
		// Fallback to embedded charts for backwards compatibility
		logger.Info("Loading charts from embedded path", "path", r.ChartsPath)
		chartFiles, err = r.loadChartsFromEmbedded(r.ChartsPath)
		if err != nil {
			logger.Error(err, "failed to load charts")
			r.updateStatusFailed(ctx, claim, "Failed to load charts: "+err.Error())
			return ctrl.Result{}, err
		}
	}

	if err := r.GiteaClient.PushFiles(ctx, repoURLs[chartsRepo], branch, chartFiles,
		"Initial charts upload by operator", "Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push charts")
		r.updateStatusFailed(ctx, claim, "Failed to push charts: "+err.Error())
		return ctrl.Result{}, err
	}

	claim.Status.ChartsUploaded = true
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Step 5: Generate root application structure in voltran repository
	logger.Info("Generating root application structure", "repo", voltranRepo)

	clusterType := claim.Spec.GitOps.ClusterType
	if clusterType == "" {
		clusterType = "nonprod"
	}

	environments := claim.Spec.GitOps.Environments
	if len(environments) == 0 {
		environments = []string{"dev", "qa", "sandbox", "staging", "prod"}
	}

	voltranFiles := r.generateVoltranStructure(claim.Spec.Organization, chartsRepo,
		clusterType, environments, branch)

	if err := r.GiteaClient.PushFiles(ctx, repoURLs[voltranRepo], branch, voltranFiles,
		"Initial GitOps structure by operator", "Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push voltran structure")
		r.updateStatusFailed(ctx, claim, "Failed to push GitOps structure: "+err.Error())
		return ctrl.Result{}, err
	}

	claim.Status.RootAppGenerated = true
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Mark as ready
	claim.Status.Phase = "Ready"
	claim.Status.Ready = true
	claim.Status.Message = "Bootstrap completed successfully"
	claim.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("BootstrapClaim reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// loadChartsFromEmbedded loads chart files from embedded directory
func (r *BootstrapReconciler) loadChartsFromEmbedded(path string) (map[string]string, error) {
	files := make(map[string]string)

	// Walk through the embedded charts directory
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Store with relative path
		relPath, err := filepath.Rel(path, filePath)
		if err != nil {
			return err
		}

		files[relPath] = string(content)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk charts directory: %w", err)
	}

	// Add README if no files found
	if len(files) == 0 {
		files["README.md"] = "# Charts Repository\n\nThis repository contains application Helm charts managed by the platform operator."
	}

	return files, nil
}

// generateVoltranStructure generates the GitOps folder structure
func (r *BootstrapReconciler) generateVoltranStructure(org, chartsRepo, clusterType string, environments []string, branch string) map[string]string {
	files := make(map[string]string)

	// README
	files["README.md"] = fmt.Sprintf(`# Voltran - GitOps Configuration Repository

This repository contains the GitOps configuration managed by the platform operator.

## Structure

- root-apps/: ArgoCD root applications
- appsets/: ApplicationSet definitions (apps & platform separated)
- environments/: Environment-specific values (applications & platform separated)

## Cluster Type: %s
`, clusterType)

	// Root applications for the cluster type (separate for apps and platform)
	appsRootAppPath := fmt.Sprintf("root-apps/%s/%s-apps-rootapp.yaml", clusterType, clusterType)
	files[appsRootAppPath] = r.generateAppsRootApp(clusterType, branch)

	platformRootAppPath := fmt.Sprintf("root-apps/%s/%s-platform-rootapp.yaml", clusterType, clusterType)
	files[platformRootAppPath] = r.generatePlatformRootApp(clusterType, branch)

	// Create directory structure for appsets
	files[fmt.Sprintf("appsets/%s/apps/.gitkeep", clusterType)] = ""
	files[fmt.Sprintf("appsets/%s/platform/.gitkeep", clusterType)] = ""

	// Create directory structure for environments
	for _, env := range environments {
		files[fmt.Sprintf("environments/%s/%s/applications/.gitkeep", clusterType, env)] = ""
		files[fmt.Sprintf("environments/%s/%s/platform/.gitkeep", clusterType, env)] = ""
	}

	return files
}

// generateAppsRootApp generates the root ArgoCD application for business applications
func (r *BootstrapReconciler) generateAppsRootApp(clusterType, branch string) string {
	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s-apps-root
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: http://gitea.gitea.svc.cluster.local:3000/platform/voltran
    path: appsets/%s/apps
    targetRevision: %s
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
      allowEmpty: false
    syncOptions:
      - CreateNamespace=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
`, clusterType, clusterType, branch)
}

// generatePlatformRootApp generates the root ArgoCD application for platform services
func (r *BootstrapReconciler) generatePlatformRootApp(clusterType, branch string) string {
	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s-platform-root
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: http://gitea.gitea.svc.cluster.local:3000/platform/voltran
    path: appsets/%s/platform
    targetRevision: %s
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
      allowEmpty: false
    syncOptions:
      - CreateNamespace=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
`, clusterType, clusterType, branch)
}

// updateStatusFailed updates the status to Failed
func (r *BootstrapReconciler) updateStatusFailed(ctx context.Context, claim *platformv1.BootstrapClaim, message string) {
	claim.Status.Phase = "Failed"
	claim.Status.Ready = false
	claim.Status.Message = message
	claim.Status.LastUpdated = metav1.Now()
	r.Status().Update(ctx, claim)
}

// SetupWithManager sets up the controller with the Manager
func (r *BootstrapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.BootstrapClaim{}).
		Complete(r)
}
