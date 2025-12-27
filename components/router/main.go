package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/signals"

	"github.com/alibaba/opensandbox/router/pkg/flag"
	"github.com/alibaba/opensandbox/router/pkg/proxy"
	"github.com/alibaba/opensandbox/router/version"
)

func main() {
	version.EchoVersion()

	flag.InitFlags()

	cfg := injection.ParseAndGetRESTConfigOrDie()
	cfg.ContentType = runtime.ContentTypeProtobuf
	cfg.UserAgent = "opensandbox-router/" + version.GitCommit

	ctx := injection.WithNamespaceScope(signals.NewContext(), flag.Namespace)
	ctx = withLogger(ctx, flag.LogLevel)

	clientset := kubernetes.NewForConfigOrDie(cfg)
	ctx = context.WithValue(ctx, kubeclient.Key{}, clientset)

	reverseProxy := proxy.NewProxy(ctx)
	http.Handle("/", reverseProxy)
	http.HandleFunc("/status.ok", proxy.Healthz)

	err := http.ListenAndServe(fmt.Sprintf(":%v", flag.Port), nil)
	if err != nil {
		log.Panicf("Error starting http server: %v", err)
	}

	panic("unreachable")
}

func withLogger(ctx context.Context, logLevel string) context.Context {
	_, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		log.Panicf("failed parsing log level from %q, %v\n", logLevel, err)
	}

	logger := logging.FromContext(ctx).Named("opensandbox.router")
	return proxy.WithLogger(ctx, logger)
}
