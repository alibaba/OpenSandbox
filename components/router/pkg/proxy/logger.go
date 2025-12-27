package proxy

import (
	"context"

	"go.uber.org/zap"
	"knative.dev/pkg/logging"
)

var Logger *zap.SugaredLogger

func WithLogger(ctx context.Context, logger *zap.SugaredLogger) context.Context {
	Logger = logger
	return logging.WithLogger(ctx, logger)
}
