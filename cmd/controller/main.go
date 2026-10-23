package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
	"github.com/envoy-gateway-rollout-controller/pkg/controller"
	"github.com/envoy-gateway-rollout-controller/pkg/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rolloutv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var webAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8081", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8082", "The address the probe endpoint binds to.")
	flag.StringVar(&webAddr, "web-bind-address", ":8080", "The address the Web Dashboard & Control API binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "envoy-gateway-rollout-lock",
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// Setup Reconciler
	reconciler := controller.NewEnvoyGatewayRolloutReconciler(mgr.GetClient(), mgr.GetScheme())
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&rolloutv1alpha1.EnvoyGatewayRollout{}).
		Complete(reconciler); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "EnvoyGatewayRollout")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	// Setup Web Dashboard & Control API Server
	var staticFS fs.FS
	// Look for UI build artifacts in ui/dist if present
	if _, err := os.Stat("ui/dist"); err == nil {
		staticFS = os.DirFS("ui/dist")
		setupLog.Info("Serving embedded Web UI from ui/dist")
	}

	ctx := ctrl.SetupSignalHandler()

	apiServer := server.NewServer(mgr.GetClient(), webAddr, staticFS)
	go func() {
		if err := apiServer.Start(ctx); err != nil {
			setupLog.Error(err, "Problem running Web API server")
		}
	}()

	fmt.Println("==========================================================")
	fmt.Println("  Envoy Gateway Rollout Controller Started Successfully   ")
	fmt.Printf("  - Web Dashboard & API : http://0.0.0.0%s\n", webAddr)
	fmt.Printf("  - Metrics Endpoint    : http://0.0.0.0%s/metrics\n", metricsAddr)
	fmt.Printf("  - Health Probe        : http://0.0.0.0%s/healthz\n", probeAddr)
	fmt.Println("==========================================================")

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
