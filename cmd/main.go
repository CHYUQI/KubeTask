package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/zapr"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/api"
	"kubetask.io/kubetask/internal/config"
	"kubetask.io/kubetask/internal/controller"
	"kubetask.io/kubetask/internal/logger"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubetaskv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to config file (YAML).")
	flag.Parse()

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		panic("failed to load configuration: " + err.Error())
	}

	log := logger.New(cfg)
	defer log.Sync()

	ctrl.SetLogger(zapr.NewLogger(log.Zap))

	setupLog := ctrl.Log.WithName("setup")

	if cfg.ConfigFile != "" {
		setupLog.Info("Loaded configuration file", "path", cfg.ConfigFile)
	}
	setupLog.Info("Logger initialized", "level", cfg.LogLevel, "format", cfg.LogFormat)

	apiAddr := fmt.Sprintf("%s:%d", cfg.APIHost, cfg.APIPort)

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Info("No Kubernetes cluster available, starting in standalone mode")
		runStandalone(apiAddr)
		return
	}

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	var tlsOpts []func(*tls.Config)
	if !cfg.EnableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{TLSOpts: tlsOpts})

	metricsServerOptions := metricsserver.Options{
		BindAddress:   cfg.MetricsAddr,
		SecureServing: cfg.SecureMetrics,
		TLSOpts:       tlsOpts,
	}

	if cfg.SecureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: cfg.HealthProbeAddr,
		LeaderElection:         cfg.EnableLeaderElection,
		LeaderElectionID:       "f652f07a.kubetask.io",
	})
	if err != nil {
		setupLog.Error(err, "Failed to create manager")
		os.Exit(1)
	}

	if err := (&controller.TaskReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to setup controller", "controller", "task")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to add health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to add ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "Failed to create Kubernetes clientset")
		os.Exit(1)
	}

	router := api.NewRouter(mgr.GetClient(), clientset, apiAddr)
	go func() {
		setupLog.Info("Starting HTTP API server", "addr", apiAddr)
		if err := router.Run(); err != nil && err != http.ErrServerClosed {
			setupLog.Error(err, "HTTP API server stopped")
		}
	}()

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}

	setupLog.Info("Shutting down HTTP API server")
	_ = router.Shutdown(10 * time.Second)
}

func runStandalone(apiAddr string) {
	engine := gin.New()
	engine.Use(gin.Recovery())

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	engine.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "no kubernetes cluster available",
			"message": "请先连接 Kubernetes 集群。确保 ~/.kube/config 存在且集群可达。",
		})
	})

	fmt.Printf("Standalone mode: listening on %s (no K8s cluster)\n", apiAddr)
	if err := engine.Run(apiAddr); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
