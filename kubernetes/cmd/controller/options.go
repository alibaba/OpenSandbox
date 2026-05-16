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
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type ConcurrencyConfig map[string]int

func (c *ConcurrencyConfig) String() string {
	if *c == nil {
		return ""
	}
	parts := make([]string, 0, len(*c))
	for k, v := range *c {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	return strings.Join(parts, ";")
}

func (c *ConcurrencyConfig) Set(value string) error {
	if *c == nil {
		*c = make(ConcurrencyConfig)
	}
	if value == "" {
		return nil
	}
	pairs := strings.Split(value, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid concurrency config format: %s, expected format: controller=value", pair)
		}
		name := strings.TrimSpace(kv[0])
		val, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			return fmt.Errorf("invalid concurrency value for %s: %v", name, err)
		}
		if val <= 0 {
			return fmt.Errorf("concurrency value must be positive for %s: %d", name, val)
		}
		(*c)[name] = val
	}
	return nil
}

func (c *ConcurrencyConfig) Get(name string, defaultVal int) int {
	if *c != nil {
		if v, ok := (*c)[name]; ok {
			return v
		}
	}
	return defaultVal
}

type controllerOptions struct {
	metricsAddr              string
	metricsCertPath          string
	metricsCertName          string
	metricsCertKey           string
	webhookCertPath          string
	webhookCertName          string
	webhookCertKey           string
	enableLeaderElection     bool
	probeAddr                string
	secureMetrics            bool
	enableHTTP2              bool
	allowWeakTLSKeyLengths   bool
	enableFileLog            bool
	logFilePath              string
	logMaxSize               int
	logMaxBackups            int
	logMaxAge                int
	logCompress              bool
	kubeClientQPS            float64
	kubeClientBurst          int
	concurrencyConfig        ConcurrencyConfig
	imageCommitterImage      string
	commitJobTimeout         time.Duration
	snapshotRegistry         string
	snapshotRegistryInsecure bool
	snapshotPushSecret       string
	resumePullSecret         string
	zapOptions               zap.Options
	legacyKlogVerbosity      string
}

func (o *controllerOptions) bindFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	fs.StringVar(&o.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&o.enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.BoolVar(&o.secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	fs.StringVar(&o.webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	fs.StringVar(&o.webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	fs.StringVar(&o.webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	fs.StringVar(&o.metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	fs.StringVar(&o.metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	fs.StringVar(&o.metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	fs.BoolVar(&o.enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	fs.BoolVar(
		&o.allowWeakTLSKeyLengths,
		"allow-weak-tls-keylengths",
		false,
		"If set, allows TLS certificates below NIST 2030 minimum key/hash lengths (not recommended).",
	)

	// Log file flags
	fs.BoolVar(&o.enableFileLog, "enable-file-log", false, "Enable log output to file")
	fs.StringVar(&o.logFilePath, "log-file-path", "/var/log/sandbox-controller/controller.log", "Path to the log file")
	fs.IntVar(&o.logMaxSize, "log-max-size", 100, "Maximum size in megabytes of the log file before it gets rotated")
	fs.IntVar(&o.logMaxBackups, "log-max-backups", 10, "Maximum number of old log files to retain")
	fs.IntVar(&o.logMaxAge, "log-max-age", 30, "Maximum number of days to retain old log files")
	fs.BoolVar(&o.logCompress, "log-compress", true, "Compress determines if the rotated log files should be compressed using gzip")
	fs.Float64Var(&o.kubeClientQPS, "kube-client-qps", 100, "QPS for Kubernetes client rate limiter.")
	fs.IntVar(&o.kubeClientBurst, "kube-client-burst", 200, "Burst for Kubernetes client rate limiter.")
	fs.Var(&o.concurrencyConfig, "concurrency", "Controller concurrency settings in format: controller1=N;controller2=M. "+
		"Available controllers: batchsandbox, pool. "+
		"Example: --concurrency='batchsandbox=32;pool=128'")

	// Image committer
	fs.StringVar(&o.imageCommitterImage, "image-committer-image", "image-committer:dev", "The image used for commit operations (contains nerdctl tool).")

	// Commit job timeout
	fs.DurationVar(&o.commitJobTimeout, "commit-job-timeout", 10*time.Minute, "The timeout duration for commit jobs.")

	fs.StringVar(&o.snapshotRegistry, "snapshot-registry", "", "OCI registry for snapshot images (e.g., registry.example.com/snapshots).")

	fs.BoolVar(&o.snapshotRegistryInsecure, "snapshot-registry-insecure", false, "Use insecure registry mode when pushing snapshot images.")

	fs.StringVar(&o.snapshotPushSecret, "snapshot-push-secret", "", "K8s Secret name for pushing snapshots to registry.")

	fs.StringVar(&o.resumePullSecret, "resume-pull-secret", "", "K8s Secret name for pulling snapshot images during resume.")

	o.zapOptions.BindFlags(fs)
	if fs.Lookup("v") == nil {
		fs.StringVar(&o.legacyKlogVerbosity, "v", "", "Deprecated compatibility flag for older controller manifests; use --zap-log-level instead.")
	}
}
