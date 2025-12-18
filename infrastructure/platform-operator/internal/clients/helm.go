package clients

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// HelmClient manages Helm operations
type HelmClient struct {
	settings  *cli.EnvSettings
	actionCfg *action.Configuration
	namespace string
}

// NewHelmClient creates a new Helm client
func NewHelmClient(namespace string) (*HelmClient, error) {
	settings := cli.New()
	actionCfg := new(action.Configuration)

	// Initialize the action configuration
	if err := actionCfg.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {
		// Log function for Helm operations
		fmt.Printf(format+"\n", v...)
	}); err != nil {
		return nil, fmt.Errorf("failed to initialize helm configuration: %w", err)
	}

	return &HelmClient{
		settings:  settings,
		actionCfg: actionCfg,
		namespace: namespace,
	}, nil
}

// AddRepository adds a Helm repository
func (c *HelmClient) AddRepository(name, url string) error {
	repoEntry := &repo.Entry{
		Name: name,
		URL:  url,
	}

	// Get the repository configuration file path
	repoFile := c.settings.RepositoryConfig

	// Load existing repositories
	f, err := repo.LoadFile(repoFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load repository file: %w", err)
	}
	if f == nil {
		f = repo.NewFile()
	}

	// Check if repository already exists
	if f.Has(name) {
		fmt.Printf("Repository %s already exists, updating...\n", name)
	}

	// Update the repository
	chartRepo, err := repo.NewChartRepository(repoEntry, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	if _, err := chartRepo.DownloadIndexFile(); err != nil {
		return fmt.Errorf("failed to download repository index: %w", err)
	}

	// Update or add the repository
	f.Update(repoEntry)

	// Save the repository file
	if err := f.WriteFile(repoFile, 0644); err != nil {
		return fmt.Errorf("failed to write repository file: %w", err)
	}

	fmt.Printf("Successfully added repository %s\n", name)
	return nil
}

// InstallChart installs a Helm chart
func (c *HelmClient) InstallChart(ctx context.Context, releaseName, chartName string, vals map[string]interface{}) (*release.Release, error) {
	client := action.NewInstall(c.actionCfg)
	client.ReleaseName = releaseName
	client.Namespace = c.namespace
	client.CreateNamespace = true
	client.Timeout = 5 * time.Minute
	client.Wait = true
	client.WaitForJobs = true

	// Find the chart
	chartPath, err := client.ChartPathOptions.LocateChart(chartName, c.settings)
	if err != nil {
		return nil, fmt.Errorf("failed to locate chart: %w", err)
	}

	// Load the chart
	chartObj, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Install the chart
	rel, err := client.RunWithContext(ctx, chartObj, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to install chart: %w", err)
	}

	fmt.Printf("Successfully installed release %s\n", releaseName)
	return rel, nil
}

// UpgradeChart upgrades an existing Helm release
func (c *HelmClient) UpgradeChart(ctx context.Context, releaseName, chartName string, vals map[string]interface{}) (*release.Release, error) {
	client := action.NewUpgrade(c.actionCfg)
	client.Namespace = c.namespace
	client.Timeout = 5 * time.Minute
	client.Wait = true
	client.WaitForJobs = true
	client.ReuseValues = true
	client.MaxHistory = 5

	// Find the chart
	chartPath, err := client.ChartPathOptions.LocateChart(chartName, c.settings)
	if err != nil {
		return nil, fmt.Errorf("failed to locate chart: %w", err)
	}

	// Load the chart
	chartObj, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Upgrade the chart
	rel, err := client.RunWithContext(ctx, releaseName, chartObj, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to upgrade chart: %w", err)
	}

	fmt.Printf("Successfully upgraded release %s\n", releaseName)
	return rel, nil
}

// UninstallChart uninstalls a Helm release
func (c *HelmClient) UninstallChart(releaseName string) error {
	client := action.NewUninstall(c.actionCfg)
	client.Timeout = 5 * time.Minute

	_, err := client.Run(releaseName)
	if err != nil {
		if err == driver.ErrReleaseNotFound {
			fmt.Printf("Release %s not found, skipping uninstall\n", releaseName)
			return nil
		}
		return fmt.Errorf("failed to uninstall release: %w", err)
	}

	fmt.Printf("Successfully uninstalled release %s\n", releaseName)
	return nil
}

// GetRelease gets information about a Helm release
func (c *HelmClient) GetRelease(releaseName string) (*release.Release, error) {
	client := action.NewGet(c.actionCfg)
	rel, err := client.Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release: %w", err)
	}
	return rel, nil
}

// ListReleases lists all Helm releases in the namespace
func (c *HelmClient) ListReleases() ([]*release.Release, error) {
	client := action.NewList(c.actionCfg)
	client.AllNamespaces = false
	client.Deployed = true
	client.Failed = true
	client.Pending = true

	releases, err := client.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	return releases, nil
}

// InstallOrUpgradeChart installs or upgrades a Helm chart
func (c *HelmClient) InstallOrUpgradeChart(ctx context.Context, releaseName, chartName string, vals map[string]interface{}) (*release.Release, error) {
	// Check if release exists
	_, err := c.GetRelease(releaseName)
	if err != nil {
		if err.Error() == fmt.Sprintf("failed to get release: release: not found") ||
		   err == driver.ErrReleaseNotFound {
			// Release doesn't exist, install it
			return c.InstallChart(ctx, releaseName, chartName, vals)
		}
		return nil, err
	}

	// Release exists, upgrade it
	return c.UpgradeChart(ctx, releaseName, chartName, vals)
}

// InstallChartFromPath installs a Helm chart from a local path
func (c *HelmClient) InstallChartFromPath(ctx context.Context, releaseName, chartPath string, vals map[string]interface{}) (*release.Release, error) {
	client := action.NewInstall(c.actionCfg)
	client.ReleaseName = releaseName
	client.Namespace = c.namespace
	client.CreateNamespace = true
	client.Timeout = 5 * time.Minute
	client.Wait = true
	client.WaitForJobs = true

	// Load the chart from path
	chartObj, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from path: %w", err)
	}

	// Install the chart
	rel, err := client.RunWithContext(ctx, chartObj, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to install chart: %w", err)
	}

	fmt.Printf("Successfully installed release %s from path %s\n", releaseName, chartPath)
	return rel, nil
}