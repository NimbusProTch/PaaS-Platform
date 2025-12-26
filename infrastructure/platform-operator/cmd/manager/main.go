package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"github.com/infraforge/platform-operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(platformv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var giteaUsername string
	var giteaToken string
	var voltranRepo string
	var gitBranch string
	var chartsPath string
	var ociBaseURL string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election")
	flag.StringVar(&giteaUsername, "gitea-username", "gitea_admin", "Gitea username")
	flag.StringVar(&giteaToken, "gitea-token", os.Getenv("GITEA_TOKEN"), "Gitea access token")
	flag.StringVar(&voltranRepo, "voltran-repo", "voltran", "GitOps voltran repository name")
	flag.StringVar(&gitBranch, "git-branch", "main", "Git branch to use")
	flag.StringVar(&chartsPath, "charts-path", "", "Path to charts directory for bootstrap")
	flag.StringVar(&ociBaseURL, "oci-base-url", "oci://ghcr.io/nimbusprotch", "Base URL for OCI chart registry")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Gitea credentials for controllers to use
	if giteaToken == "" {
		setupLog.Info("Gitea token not provided, GitOps features will be disabled")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "platform-operator.infraforge.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup controllers with credentials from environment
	if giteaToken != "" {
		// Bootstrap controller - creates GiteaClient from claim
		if err = (&controller.BootstrapReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			GiteaUsername: giteaUsername,
			GiteaToken:    giteaToken,
			ChartsPath:    chartsPath,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Bootstrap")
			os.Exit(1)
		}

		// ApplicationClaim GitOps controller - uses claim values
		if err = (&controller.ApplicationClaimGitOpsReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			GiteaUsername: giteaUsername,
			GiteaToken:    giteaToken,
			VoltranRepo:   voltranRepo,
			Branch:        gitBranch,
			OCIBaseURL:    ociBaseURL,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ApplicationClaimGitOps")
			os.Exit(1)
		}

		// PlatformApplicationClaim controller - uses claim values
		if err = (&controller.PlatformApplicationClaimReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			GiteaUsername: giteaUsername,
			GiteaToken:    giteaToken,
			VoltranRepo:   voltranRepo,
			Branch:        gitBranch,
			OCIBaseURL:    ociBaseURL,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "PlatformApplicationClaim")
			os.Exit(1)
		}

		setupLog.Info("All controllers registered successfully with GitOps enabled")
	}

	// Add health and readiness checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
