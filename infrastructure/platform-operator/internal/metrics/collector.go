package metrics

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
)

var (
	// ApplicationClaim metrics
	applicationClaimsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_application_claims_total",
			Help: "Total number of ApplicationClaims in the cluster",
		},
		[]string{"namespace", "environment", "status"},
	)

	applicationClaimsReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_application_claims_ready",
			Help: "Number of ApplicationClaims in ready state",
		},
		[]string{"namespace", "environment"},
	)

	// Application metrics
	applicationsDeployed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_applications_deployed_total",
			Help: "Total number of applications deployed by the operator",
		},
		[]string{"namespace", "name", "version"},
	)

	applicationReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_application_replicas",
			Help: "Number of replicas for each application",
		},
		[]string{"namespace", "name"},
	)

	// Component metrics
	componentsDeployed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_components_deployed_total",
			Help: "Total number of components deployed by the operator",
		},
		[]string{"namespace", "type", "name", "version"},
	)

	// Reconciliation metrics
	reconciliationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "platform_operator_reconciliations_total",
			Help: "Total number of reconciliation attempts",
		},
		[]string{"namespace", "name", "result"},
	)

	reconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "platform_operator_reconciliation_duration_seconds",
			Help:    "Duration of reconciliation in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"namespace", "name"},
	)

	reconciliationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "platform_operator_reconciliation_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"namespace", "name", "error_type"},
	)

	// Resource metrics
	namespacesManaged = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "platform_operator_namespaces_managed_total",
			Help: "Total number of namespaces managed by the operator",
		},
	)

	deploymentsManaged = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_deployments_managed_total",
			Help: "Total number of deployments managed by the operator",
		},
		[]string{"namespace"},
	)

	servicesManaged = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_services_managed_total",
			Help: "Total number of services managed by the operator",
		},
		[]string{"namespace"},
	)

	// Helm metrics
	helmReleasesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_helm_releases_total",
			Help: "Total number of Helm releases managed by the operator",
		},
		[]string{"namespace", "chart", "status"},
	)

	helmOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "platform_operator_helm_operations_total",
			Help: "Total number of Helm operations performed",
		},
		[]string{"namespace", "operation", "result"},
	)

	// GitHub metrics
	githubReleasesDownloaded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "platform_operator_github_releases_downloaded_total",
			Help: "Total number of GitHub releases downloaded",
		},
		[]string{"repository", "version"},
	)

	// Health metrics
	operatorHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_health",
			Help: "Health status of the operator (1 = healthy, 0 = unhealthy)",
		},
		[]string{"component"},
	)

	operatorInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_operator_info",
			Help: "Information about the operator",
		},
		[]string{"version", "git_commit", "build_date"},
	)
)

func init() {
	// Register metrics with controller-runtime metrics registry
	metrics.Registry.MustRegister(
		applicationClaimsTotal,
		applicationClaimsReady,
		applicationsDeployed,
		applicationReplicas,
		componentsDeployed,
		reconciliationsTotal,
		reconciliationDuration,
		reconciliationErrors,
		namespacesManaged,
		deploymentsManaged,
		servicesManaged,
		helmReleasesTotal,
		helmOperationsTotal,
		githubReleasesDownloaded,
		operatorHealth,
		operatorInfo,
	)

	// Set operator info metric
	operatorInfo.WithLabelValues(
		"v1.0.0",     // version
		"unknown",    // git commit
		"2024-12-18", // build date
	).Set(1)

	// Set initial health status
	operatorHealth.WithLabelValues("controller").Set(1)
	operatorHealth.WithLabelValues("webhook").Set(1)
	operatorHealth.WithLabelValues("metrics").Set(1)
}

// MetricsCollector collects metrics for the operator
type MetricsCollector struct {
	client client.Client
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(client client.Client) *MetricsCollector {
	return &MetricsCollector{
		client: client,
	}
}

// CollectApplicationClaimMetrics collects metrics for ApplicationClaims
func (c *MetricsCollector) CollectApplicationClaimMetrics(ctx context.Context) error {
	// List all ApplicationClaims
	claims := &platformv1.ApplicationClaimList{}
	if err := c.client.List(ctx, claims); err != nil {
		return fmt.Errorf("failed to list ApplicationClaims: %w", err)
	}

	// Reset metrics
	applicationClaimsTotal.Reset()
	applicationClaimsReady.Reset()
	applicationsDeployed.Reset()
	applicationReplicas.Reset()
	componentsDeployed.Reset()

	// Collect metrics for each claim
	for _, claim := range claims.Items {
		status := "pending"
		if claim.Status.Ready {
			status = "ready"
		}

		// Application claim metrics
		applicationClaimsTotal.WithLabelValues(
			claim.Namespace,
			claim.Spec.Environment,
			status,
		).Inc()

		if claim.Status.Ready {
			applicationClaimsReady.WithLabelValues(
				claim.Namespace,
				claim.Spec.Environment,
			).Inc()
		}

		// Application metrics
		for _, app := range claim.Spec.Applications {
			applicationsDeployed.WithLabelValues(
				claim.Spec.Namespace,
				app.Name,
				app.Version,
			).Inc()

			if app.Replicas > 0 {
				applicationReplicas.WithLabelValues(
					claim.Spec.Namespace,
					app.Name,
				).Set(float64(app.Replicas))
			}
		}

		// Component metrics
		for _, comp := range claim.Spec.Components {
			componentsDeployed.WithLabelValues(
				claim.Spec.Namespace,
				comp.Type,
				comp.Name,
				comp.Version,
			).Inc()
		}
	}

	return nil
}

// RecordReconciliation records metrics for a reconciliation
func RecordReconciliation(namespace, name string, duration float64, err error) {
	// Record duration
	reconciliationDuration.WithLabelValues(namespace, name).Observe(duration)

	// Record result
	result := "success"
	if err != nil {
		result = "error"
		reconciliationErrors.WithLabelValues(namespace, name, getErrorType(err)).Inc()
	}
	reconciliationsTotal.WithLabelValues(namespace, name, result).Inc()
}

// RecordHelmOperation records metrics for a Helm operation
func RecordHelmOperation(namespace, operation string, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	helmOperationsTotal.WithLabelValues(namespace, operation, result).Inc()
}

// RecordGitHubDownload records a GitHub release download
func RecordGitHubDownload(repository, version string) {
	githubReleasesDownloaded.WithLabelValues(repository, version).Inc()
}

// UpdateNamespacesManaged updates the count of managed namespaces
func UpdateNamespacesManaged(count int) {
	namespacesManaged.Set(float64(count))
}

// UpdateDeploymentsManaged updates the count of managed deployments
func UpdateDeploymentsManaged(namespace string, count int) {
	deploymentsManaged.WithLabelValues(namespace).Set(float64(count))
}

// UpdateServicesManaged updates the count of managed services
func UpdateServicesManaged(namespace string, count int) {
	servicesManaged.WithLabelValues(namespace).Set(float64(count))
}

// UpdateHelmReleases updates Helm release metrics
func UpdateHelmReleases(namespace, chart, status string, count int) {
	helmReleasesTotal.WithLabelValues(namespace, chart, status).Set(float64(count))
}

// SetOperatorHealth sets the health status of an operator component
func SetOperatorHealth(component string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	operatorHealth.WithLabelValues(component).Set(value)
}

// getErrorType returns a categorized error type for metrics
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}

	// Categorize common error types
	errStr := err.Error()
	switch {
	case contains(errStr, "not found"):
		return "not_found"
	case contains(errStr, "already exists"):
		return "already_exists"
	case contains(errStr, "timeout"):
		return "timeout"
	case contains(errStr, "connection refused"):
		return "connection"
	case contains(errStr, "permission denied"):
		return "permission"
	case contains(errStr, "validation"):
		return "validation"
	default:
		return "unknown"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
