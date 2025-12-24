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
	"github.com/infraforge/platform-operator/pkg/gitea"
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
	var giteaURL string
	var giteaUsername string
	var giteaToken string
	var giteaOrg string
	var voltranRepo string
	var gitBranch string
	var chartsPath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election")
	flag.StringVar(&giteaURL, "gitea-url", "http://gitea.gitea.svc.cluster.local:3000", "Gitea server URL")
	flag.StringVar(&giteaUsername, "gitea-username", "platform", "Gitea username")
	flag.StringVar(&giteaToken, "gitea-token", os.Getenv("GITEA_TOKEN"), "Gitea access token")
	flag.StringVar(&giteaOrg, "gitea-org", "platform", "Gitea organization")
	flag.StringVar(&voltranRepo, "voltran-repo", "voltran", "GitOps voltran repository name")
	flag.StringVar(&gitBranch, "git-branch", "main", "Git branch to use")
	flag.StringVar(&chartsPath, "charts-path", "", "Path to charts directory for bootstrap")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Initialize Gitea client
	var giteaClient *gitea.Client
	if giteaToken != "" {
		giteaClient = gitea.NewClient(giteaURL, giteaUsername, giteaToken)
		setupLog.Info("Gitea client initialized", "url", giteaURL, "org", giteaOrg)
	} else {
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

	// Setup Bootstrap controller
	if giteaClient != nil {
		if err = (&controller.BootstrapReconciler{
			Client:      mgr.GetClient(),
			Scheme:      mgr.GetScheme(),
			GiteaClient: giteaClient,
			ChartsPath:  chartsPath,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Bootstrap")
			os.Exit(1)
		}

		// Setup ApplicationClaim GitOps controller
		if err = (&controller.ApplicationClaimGitOpsReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			GiteaClient:  giteaClient,
			Organization: giteaOrg,
			VoltranRepo:  voltranRepo,
			Branch:       gitBranch,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ApplicationClaimGitOps")
			os.Exit(1)
		}

		// Setup PlatformApplicationClaim controller
		if err = (&controller.PlatformApplicationClaimReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			GiteaClient:  giteaClient,
			Organization: giteaOrg,
			VoltranRepo:  voltranRepo,
			Branch:       gitBranch,
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
