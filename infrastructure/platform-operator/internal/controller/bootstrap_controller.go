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

	// Gitea credentials - client created dynamically from claim
	GiteaUsername string
	GiteaToken    string

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

	// Create GiteaClient dynamically from claim
	giteaClient := gitea.NewClient(claim.Spec.GiteaURL, r.GiteaUsername, r.GiteaToken)

	// Update status to Bootstrapping
	claim.Status.Phase = "Bootstrapping"
	claim.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Step 1: Create organization
	logger.Info("Creating Gitea organization", "org", claim.Spec.Organization)
	if err := giteaClient.CreateOrganization(ctx, claim.Spec.Organization, "Platform organization"); err != nil {
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
		_, err := giteaClient.CreateRepository(ctx, claim.Spec.Organization, gitea.CreateRepoOptions{
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
		cloneURL := giteaClient.ConstructCloneURL(claim.Spec.Organization, repoName)
		repoURLs[repoName] = cloneURL
		logger.Info("Repository created", "name", repoName, "url", cloneURL)
	}

	claim.Status.RepositoriesCreated = true
	claim.Status.RepositoryURLs = repoURLs
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Step 3: Upload charts to charts repository (contains both app and platform templates)
	// Note: In OCI mode, charts are NOT uploaded to Gitea - they live in OCI registry only
	// Bootstrap only creates the GitOps structure in voltran repo

	var chartFiles map[string]string

	// Check if external charts repository is specified
	if claim.Spec.ChartsRepository != nil {
		repoType := claim.Spec.ChartsRepository.Type
		if repoType == "" {
			repoType = "git" // Default to git for backwards compatibility
		}

		logger.Info("Chart repository mode", "url", claim.Spec.ChartsRepository.URL, "type", repoType)

		if repoType == "oci" {
			// In OCI mode, skip chart upload to Gitea
			// Charts are pulled directly from OCI registry by ArgoCD
			logger.Info("OCI mode: Skipping chart upload to Gitea (charts live in OCI registry)")
			chartFiles = make(map[string]string) // Empty files - we'll only create GitOps structure
			claim.Status.ChartsUploaded = true // Mark as uploaded (skipped)
		} else {
			// Clone from Git repository
			chartsBranch := claim.Spec.ChartsRepository.Branch
			if chartsBranch == "" {
				chartsBranch = "main"
			}
			chartsPath := claim.Spec.ChartsRepository.Path

			logger.Info("Cloning charts from Git repository", "branch", chartsBranch, "path", chartsPath)
			var err error
			chartFiles, err = giteaClient.CloneAndExtractFiles(ctx, claim.Spec.ChartsRepository.URL, chartsBranch, chartsPath)
			if err != nil {
				logger.Error(err, "failed to clone charts from Git repository")
				r.updateStatusFailed(ctx, claim, "Failed to clone charts: "+err.Error())
				return ctrl.Result{}, err
			}
		}
	} else {
		// Fallback to embedded charts for backwards compatibility
		logger.Info("Loading charts from embedded path", "path", r.ChartsPath)
		var err error
		chartFiles, err = r.loadChartsFromEmbedded(r.ChartsPath)
		if err != nil {
			logger.Error(err, "failed to load charts")
			r.updateStatusFailed(ctx, claim, "Failed to load charts: "+err.Error())
			return ctrl.Result{}, err
		}
	}

	if err := giteaClient.PushFiles(ctx, repoURLs[chartsRepo], branch, chartFiles,
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
		clusterType, environments, branch, voltranRepo, claim.Spec.GiteaURL)

	if err := giteaClient.PushFiles(ctx, repoURLs[voltranRepo], branch, voltranFiles,
		"Initial GitOps structure by operator", "Platform Operator", "operator@platform.local"); err != nil {
		logger.Error(err, "failed to push voltran structure")
		r.updateStatusFailed(ctx, claim, "Failed to push GitOps structure: "+err.Error())
		return ctrl.Result{}, err
	}

	claim.Status.RootAppGenerated = true
	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	// Generate ArgoCD setup manifests in the GitOps repo
	logger.Info("Generating ArgoCD setup manifests")
	if err := r.generateArgoCDSetup(ctx, claim, giteaClient, repoURLs[voltranRepo], branch); err != nil {
		logger.Error(err, "failed to generate ArgoCD setup manifests")
		// Don't fail the whole reconciliation, just log the error
		claim.Status.Message = fmt.Sprintf("Bootstrap completed but ArgoCD setup generation failed: %v", err)
	} else {
		claim.Status.Message = "Bootstrap completed successfully - Apply argocd-setup manifests to complete"
	}

	// Mark as ready
	claim.Status.Phase = "Ready"
	claim.Status.Ready = true
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
func (r *BootstrapReconciler) generateVoltranStructure(org, chartsRepo, clusterType string, environments []string, branch, voltranRepo, giteaURL string) map[string]string {
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
	files[appsRootAppPath] = r.generateAppsRootApp(org, voltranRepo, clusterType, branch, giteaURL)

	platformRootAppPath := fmt.Sprintf("root-apps/%s/%s-platform-rootapp.yaml", clusterType, clusterType)
	files[platformRootAppPath] = r.generatePlatformRootApp(org, voltranRepo, clusterType, branch, giteaURL)

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
func (r *BootstrapReconciler) generateAppsRootApp(org, voltranRepo, clusterType, branch, giteaURL string) string {
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
    repoURL: %s/%s/%s
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
`, clusterType, giteaURL, org, voltranRepo, clusterType, branch)
}

// generatePlatformRootApp generates the root ArgoCD application for platform services
func (r *BootstrapReconciler) generatePlatformRootApp(org, voltranRepo, clusterType, branch, giteaURL string) string {
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
    repoURL: %s/%s/%s
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
`, clusterType, giteaURL, org, voltranRepo, clusterType, branch)
}

// updateStatusFailed updates the status to Failed
func (r *BootstrapReconciler) updateStatusFailed(ctx context.Context, claim *platformv1.BootstrapClaim, message string) {
	claim.Status.Phase = "Failed"
	claim.Status.Ready = false
	claim.Status.Message = message
	claim.Status.LastUpdated = metav1.Now()
	r.Status().Update(ctx, claim)
}

// generateArgoCDSetup generates ArgoCD setup manifests in the GitOps repo
func (r *BootstrapReconciler) generateArgoCDSetup(ctx context.Context, claim *platformv1.BootstrapClaim, giteaClient *gitea.Client, voltranURL, branch string) error {
	logger := log.FromContext(ctx)

	clusterType := claim.Spec.GitOps.ClusterType
	if clusterType == "" {
		clusterType = "nonprod"
	}

	// Generate ArgoCD setup manifests
	setupFiles := make(map[string]string)

	// 1. Repository secret for Gitea
	setupFiles["argocd-setup/01-repo-secret.yaml"] = fmt.Sprintf(`# ArgoCD Repository Secret for Gitea
# Apply this to enable ArgoCD to pull from Gitea
apiVersion: v1
kind: Secret
metadata:
  name: gitea-repo
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
type: Opaque
stringData:
  type: git
  url: %s/%s/voltran
  username: %s
  password: %s
`, claim.Spec.GiteaURL, claim.Spec.Organization, r.GiteaUsername, r.GiteaToken)

	// 2. OCI registry credentials for GHCR (for pulling Helm charts)
	setupFiles["argocd-setup/02-helm-oci-secret.yaml"] = `# ArgoCD Helm OCI Registry Credentials
# Apply this to enable ArgoCD to pull Helm charts from GHCR
# NOTE: Replace GITHUB_TOKEN with your actual token
apiVersion: v1
kind: Secret
metadata:
  name: helm-oci-creds
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
type: Opaque
stringData:
  type: helm
  url: oci://ghcr.io/infraforge
  username: infraforge
  password: GITHUB_TOKEN  # Replace with actual GitHub token
  enableOCI: "true"
`

	// 3. GitHub token secret for image pulls
	setupFiles["argocd-setup/03-github-token-secret.yaml"] = `# GitHub Token Secret for Image Pulls from GHCR
# Apply this to enable pulling container images from GitHub Container Registry
# NOTE: Replace GITHUB_TOKEN with your actual token
apiVersion: v1
kind: Secret
metadata:
  name: github-token
  namespace: platform-operator-system
type: kubernetes.io/dockerconfigjson
stringData:
  .dockerconfigjson: |
    {
      "auths": {
        "ghcr.io": {
          "username": "infraforge",
          "password": "GITHUB_TOKEN",
          "auth": "BASE64_ENCODED_USERNAME:TOKEN"
        }
      }
    }
---
# ImagePullSecret for pulling images from GHCR
# This will be used by the platform-operator deployment
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-pull-secret
  namespace: platform-operator-system
type: kubernetes.io/dockerconfigjson
stringData:
  .dockerconfigjson: |
    {
      "auths": {
        "ghcr.io": {
          "username": "infraforge",
          "password": "GITHUB_TOKEN",
          "auth": "BASE64_ENCODED_USERNAME:TOKEN"
        }
      }
    }
`

	// 4. README with instructions
	setupFiles["argocd-setup/README.md"] = fmt.Sprintf(`# ArgoCD Setup Manifests

These manifests were automatically generated by the Platform Operator to set up ArgoCD for your GitOps workflow.

## Prerequisites

1. Ensure ArgoCD is installed in your cluster:

    kubectl create namespace argocd
    kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

2. Ensure you have a GitHub token with package:read permissions for pulling Helm charts from GHCR

## Setup Steps

1. **Update GitHub tokens in the manifests**:
   - In 02-helm-oci-secret.yaml: Replace GITHUB_TOKEN with your actual GitHub token
   - In 03-github-token-secret.yaml: Replace GITHUB_TOKEN and BASE64_ENCODED_USERNAME:TOKEN
     (You can generate the base64 auth with: echo -n "infraforge:YOUR_GITHUB_TOKEN" | base64)

2. **Apply the secret manifests**:

    kubectl apply -f argocd-setup/

   Or apply individually:

    kubectl apply -f argocd-setup/01-repo-secret.yaml
    kubectl apply -f argocd-setup/02-helm-oci-secret.yaml
    kubectl apply -f argocd-setup/03-github-token-secret.yaml

3. **Deploy the root applications**:

    # The root apps are already generated in voltran/root-apps/%s/
    kubectl apply -f root-apps/%s/

4. **Verify the setup**:

    # Check if secrets are created
    kubectl get secrets -n argocd | grep -E "gitea-repo|helm-oci-creds"
    kubectl get secrets -n platform-operator-system | grep -E "github-token|ghcr-pull-secret"

    # Check if root applications are created and synced
    kubectl get applications -n argocd

## What These Manifests Do

- **01-repo-secret.yaml**: Configures ArgoCD to authenticate with your Gitea repository
- **02-helm-oci-secret.yaml**: Configures ArgoCD to pull Helm charts from GitHub Container Registry
- **03-github-token-secret.yaml**: Configures image pull secrets for pulling container images from GHCR
- **root-apps/%s/*.yaml**: Root applications that watch for ApplicationSets (already in voltran repo)

## Next Steps

After applying these manifests:

1. Create ApplicationClaims to deploy your applications:

    kubectl apply -f deployments/dev/apps-claim.yaml
    kubectl apply -f deployments/dev/platform-infrastructure-claim.yaml

2. Monitor the operator logs:

    kubectl logs -n platform-operator-system -l control-plane=controller-manager -f

3. Check ArgoCD UI to see your applications being deployed:

    kubectl port-forward svc/argocd-server -n argocd 8080:443

   Then visit https://localhost:8080

## Troubleshooting

- If applications show as "Unknown" in ArgoCD, ensure the OCI registry credentials are correct
- If sync fails, check that the Gitea repository secret has the correct credentials
- For image pull issues, ensure the GitHub token secret exists in the platform-operator-system namespace

Generated by Platform Operator at $(date)
`, clusterType, clusterType, clusterType)

	// Push the setup files to Gitea
	if err := giteaClient.PushFiles(ctx, voltranURL, branch, setupFiles,
		"Add ArgoCD setup manifests", "Platform Operator", "operator@platform.local"); err != nil {
		return fmt.Errorf("failed to push ArgoCD setup manifests: %w", err)
	}

	logger.Info("Successfully generated ArgoCD setup manifests in voltran/argocd-setup/")
	logger.Info("To complete setup, apply the manifests: kubectl apply -f argocd-setup/")

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *BootstrapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.BootstrapClaim{}).
		Complete(r)
}
