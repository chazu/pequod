/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	cuembed "github.com/chazu/pequod/cue"
	"github.com/chazu/pequod/internal/controller"
	"github.com/chazu/pequod/pkg/platformloader"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// Config holds the command-line configuration
type Config struct {
	MetricsAddr          string
	MetricsCertPath      string
	MetricsCertName      string
	MetricsCertKey       string
	WebhookCertPath      string
	WebhookCertName      string
	WebhookCertKey       string
	ProbeAddr            string
	EnableLeaderElection bool
	SecureMetrics        bool
	EnableHTTP2          bool
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	utilruntime.Must(platformv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// parseFlags parses command-line flags and returns configuration
func parseFlags() Config {
	cfg := Config{}
	flag.StringVar(&cfg.MetricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&cfg.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&cfg.EnableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&cfg.SecureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&cfg.WebhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&cfg.WebhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&cfg.WebhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&cfg.MetricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&cfg.MetricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&cfg.MetricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&cfg.EnableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	return cfg
}

// getTLSOptions returns TLS configuration options
func getTLSOptions(enableHTTP2 bool) []func(*tls.Config) {
	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		// Disable HTTP/2 to prevent CVEs (GHSA-qppj-fm5r-hxr3, GHSA-4374-p667-p6c8)
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			setupLog.Info("disabling http/2")
			c.NextProtos = []string{"http/1.1"}
		})
	}
	return tlsOpts
}

// newWebhookServer creates a webhook server with the given configuration
func newWebhookServer(cfg Config, tlsOpts []func(*tls.Config)) webhook.Server {
	opts := webhook.Options{TLSOpts: tlsOpts}
	if len(cfg.WebhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher",
			"path", cfg.WebhookCertPath, "cert", cfg.WebhookCertName, "key", cfg.WebhookCertKey)
		opts.CertDir = cfg.WebhookCertPath
		opts.CertName = cfg.WebhookCertName
		opts.KeyName = cfg.WebhookCertKey
	}
	return webhook.NewServer(opts)
}

// newMetricsServerOptions creates metrics server options with the given configuration
func newMetricsServerOptions(cfg Config, tlsOpts []func(*tls.Config)) metricsserver.Options {
	opts := metricsserver.Options{
		BindAddress:   cfg.MetricsAddr,
		SecureServing: cfg.SecureMetrics,
		TLSOpts:       tlsOpts,
	}
	if cfg.SecureMetrics {
		opts.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if len(cfg.MetricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher",
			"path", cfg.MetricsCertPath, "cert", cfg.MetricsCertName, "key", cfg.MetricsCertKey)
		opts.CertDir = cfg.MetricsCertPath
		opts.CertName = cfg.MetricsCertName
		opts.KeyName = cfg.MetricsCertKey
	}
	return opts
}

// setupControllers sets up all controllers with the manager
func setupControllers(mgr ctrl.Manager) error {
	// Setup ResourceGraph controller (executes rendered graphs)
	if err := (&controller.ResourceGraphReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup platform loader with K8s client and embedded CUE modules
	loader := platformloader.NewLoaderWithConfig(platformloader.LoaderConfig{
		K8sClient:       mgr.GetClient(),
		EmbeddedFS:      cuembed.PlatformFS,
		EmbeddedRootDir: cuembed.PlatformDir,
	})
	renderer := platformloader.NewRenderer(loader)

	// Setup Transform controller (generates CRDs from Transform definitions)
	if err := (&controller.TransformReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		PlatformLoader: loader,
		Recorder:       mgr.GetEventRecorderFor("transform-controller"),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup Platform Instance controller (watches generated CRDs and creates ResourceGraphs)
	if err := (&controller.PlatformInstanceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("platforminstance-controller"),
		Renderer: renderer,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}

func main() {
	cfg := parseFlags()
	tlsOpts := getTLSOptions(cfg.EnableHTTP2)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                newMetricsServerOptions(cfg, tlsOpts),
		WebhookServer:          newWebhookServer(cfg, tlsOpts),
		HealthProbeBindAddress: cfg.ProbeAddr,
		LeaderElection:         cfg.EnableLeaderElection,
		LeaderElectionID:       "da711d0c.platform.example.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := setupControllers(mgr); err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

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
