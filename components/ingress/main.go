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
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/signals"

	"github.com/alibaba/opensandbox/ingress/pkg/flag"
	"github.com/alibaba/opensandbox/ingress/pkg/proxy"
	"github.com/alibaba/opensandbox/ingress/pkg/sandbox"
	"github.com/alibaba/opensandbox/ingress/version"
)

func main() {
	version.EchoVersion()

	flag.InitFlags()
	if flag.Namespace == "" {
		log.Panicf("'-namespace' not set.")
	}

	cfg := injection.ParseAndGetRESTConfigOrDie()
	cfg.ContentType = runtime.ContentTypeProtobuf
	cfg.UserAgent = "opensandbox-ingress/" + version.GitCommit

	ctx := signals.NewContext()
	ctx = withLogger(ctx, flag.LogLevel)

	// Create sandbox provider factory
	providerFactory := sandbox.NewProviderFactory(
		cfg,
		flag.Namespace,
		time.Second*30, // resync period
	)

	// Create sandbox provider based on provider type
	sandboxProvider, err := providerFactory.CreateProvider(flag.ProviderType)
	if err != nil {
		log.Panicf("Failed to create sandbox provider: %v", err)
	}

	// Start provider (includes cache sync)
	if err := sandboxProvider.Start(ctx); err != nil {
		log.Panicf("Failed to start sandbox provider: %v", err)
	}

	// Create reverse proxy with sandbox provider
	reverseProxy := proxy.NewProxy(ctx, sandboxProvider)
	http.Handle("/", reverseProxy)
	http.HandleFunc("/status.ok", proxy.Healthz)

	if err := http.ListenAndServe(fmt.Sprintf(":%v", flag.Port), nil); err != nil {
		log.Panicf("Error starting http server: %v", err)
	}

	panic("unreachable")
}

func withLogger(ctx context.Context, logLevel string) context.Context {
	_, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		log.Panicf("failed parsing log level from %q, %v\n", logLevel, err)
	}

	logger := logging.FromContext(ctx).Named("opensandbox.ingress")
	return proxy.WithLogger(ctx, logger)
}
