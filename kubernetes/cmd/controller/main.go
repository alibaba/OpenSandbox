// Copyright 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/controller"
	poolassign "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/controller/poolassign"
	cryptoutil "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/crypto"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/fieldindex"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/logging"
	// +kubebuilder:scaffold:imports
)

const (
	defaultBatchSandboxConcurrency = 32
	defaultPoolConcurrency         = 16
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// getKindFromType returns the Kind name for a given runtime.Object using the scheme.
// It panics if the object's kind cannot be determined.
func getKindFromType(obj runtime.Object) string {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		panic(fmt.Sprintf("failed to get kind for object %T: %v", obj, err))
	}
	if len(gvks) == 0 {
		panic(fmt.Sprintf("no kind registered for object %T", obj))
	}
	return gvks[0].Kind
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(sandboxv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var tlsOpts []func(*tls.Config)

	options := &controllerOptions{}
	options.bindFlags(flag.CommandLine)

	flag.Parse()

	// Setup logger with file rotation support
	logOpts := logging.Options{
		Development:      options.zapOptions.Development,
		EnableFileOutput: options.enableFileLog,
		LogFilePath:      options.logFilePath,
		MaxSize:          options.logMaxSize,
		MaxBackups:       options.logMaxBackups,
		MaxAge:           options.logMaxAge,
		Compress:         options.logCompress,
		ZapOptions:       options.zapOptions,
	}

	logger := logging.NewLoggerWithZapOptions(logOpts)
	ctrl.SetLogger(logger)
	if options.legacyKlogVerbosity != "" {
		setupLog.Info("deprecated --v flag ignored; use --zap-log-level instead", "v", options.legacyKlogVerbosity)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts = append(tlsOpts, func(c *tls.Config) {
		c.MinVersion = tls.VersionTLS12
	})

	if !options.enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(options.webhookCertPath) > 0 {
		webhookCertFile := filepath.Join(options.webhookCertPath, options.webhookCertName)
		webhookKeyFile := filepath.Join(options.webhookCertPath, options.webhookCertKey)
		if !options.allowWeakTLSKeyLengths {
			if err := cryptoutil.ValidateCertificateKeyPair(webhookCertFile, webhookKeyFile); err != nil {
				setupLog.Error(err, "Webhook certificate does not meet NIST minimum key/hash requirements",
					"webhook-cert-file", webhookCertFile, "webhook-key-file", webhookKeyFile)
				os.Exit(1)
			}
		}

		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", options.webhookCertPath, "webhook-cert-name", options.webhookCertName, "webhook-cert-key", options.webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			webhookCertFile,
			webhookKeyFile,
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert, err := webhookCertWatcher.GetCertificate(chi)
				if err != nil {
					return nil, err
				}
				if options.allowWeakTLSKeyLengths {
					return cert, nil
				}
				if err := cryptoutil.ValidateTLSCertificate(webhookCertFile, cert); err != nil {
					return nil, err
				}
				return cert, nil
			}
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   options.metricsAddr,
		SecureServing: options.secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if options.secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(options.metricsCertPath) > 0 {
		metricsCertFile := filepath.Join(options.metricsCertPath, options.metricsCertName)
		metricsKeyFile := filepath.Join(options.metricsCertPath, options.metricsCertKey)
		if !options.allowWeakTLSKeyLengths && options.metricsAddr != "0" && options.secureMetrics {
			if err := cryptoutil.ValidateCertificateKeyPair(metricsCertFile, metricsKeyFile); err != nil {
				setupLog.Error(err, "Metrics certificate does not meet NIST minimum key/hash requirements",
					"metrics-cert-file", metricsCertFile, "metrics-key-file", metricsKeyFile)
				os.Exit(1)
			}
		}

		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", options.metricsCertPath, "metrics-cert-name", options.metricsCertName, "metrics-cert-key", options.metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			metricsCertFile,
			metricsKeyFile,
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert, err := metricsCertWatcher.GetCertificate(chi)
				if err != nil {
					return nil, err
				}
				if options.allowWeakTLSKeyLengths {
					return cert, nil
				}
				if err := cryptoutil.ValidateTLSCertificate(metricsCertFile, cert); err != nil {
					return nil, err
				}
				return cert, nil
			}
		})
	}

	config := ctrl.GetConfigOrDie()
	// Set client rate limiter if specified
	if options.kubeClientQPS > 0 {
		config.QPS = float32(options.kubeClientQPS)
	}
	if options.kubeClientBurst > 0 {
		config.Burst = options.kubeClientBurst
	}

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: options.probeAddr,
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       "2fa1c467.opensandbox.io",
		// LeaderElectionReleaseOnCancel causes the leader to voluntarily release the lease
		// when the Manager is stopped, allowing a new leader to acquire it without waiting
		// for the full LeaseDuration. This is safe because main() exits immediately after
		// mgr.Start() returns and performs no post-stop cleanup.
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	setupLog.Info("register field index")
	if err := fieldindex.RegisterFieldIndexes(mgr.GetCache()); err != nil {
		setupLog.Error(err, "failed to register field index")
		os.Exit(1)
	}

	var (
		batchSandboxKindName = strings.ToLower(getKindFromType(&sandboxv1alpha1.BatchSandbox{}))
		poolKindName         = strings.ToLower(getKindFromType(&sandboxv1alpha1.Pool{}))
	)
	batchSandboxConcurrency := options.concurrencyConfig.Get(batchSandboxKindName, defaultBatchSandboxConcurrency)
	poolConcurrency := options.concurrencyConfig.Get(poolKindName, defaultPoolConcurrency)
	setupLog.Info("controller concurrency configured", batchSandboxKindName, batchSandboxConcurrency, poolKindName, poolConcurrency)

	profileStore := poolassign.NewProfileStore()
	_ = profileStore.LoadDefault()
	if err := profileStore.SetupWithManager(mgr, os.Getenv("POD_NAMESPACE")); err != nil {
		setupLog.Error(err, "failed to setup pool assign profiles ConfigMap watch")
		os.Exit(1)
	}

	if err := (&controller.BatchSandboxReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		Recorder:         mgr.GetEventRecorderFor("batchsandbox-controller"),
		ResumePullSecret: options.resumePullSecret,
		ProfileStore:     profileStore,
	}).SetupWithManager(mgr, batchSandboxConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BatchSandbox")
		os.Exit(1)
	}
	if err := (&controller.PoolReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("pool-controller"),
		Allocator:  controller.NewDefaultAllocator(mgr.GetClient()),
		RestConfig: mgr.GetConfig(),
	}).SetupWithManager(mgr, poolConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pool")
		os.Exit(1)
	}
	if err := (&controller.SandboxSnapshotReconciler{
		Client:                   mgr.GetClient(),
		Scheme:                   mgr.GetScheme(),
		Recorder:                 mgr.GetEventRecorderFor("sandboxsnapshot-controller"),
		ImageCommitterImage:      options.imageCommitterImage,
		CommitJobTimeout:         options.commitJobTimeout,
		SnapshotRegistry:         options.snapshotRegistry,
		SnapshotRegistryInsecure: options.snapshotRegistryInsecure,
		SnapshotPushSecret:       options.snapshotPushSecret,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SandboxSnapshot")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

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
