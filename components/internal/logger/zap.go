// Copyright 2026 Alibaba Group Holding Ltd.
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

package logger

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config is the minimal configuration to align execd/ingress defaults.
// - JSON encoding, ISO8601 time
// - Caller/stacktrace disabled
// - Stdout as default output
// - Level defaults to info
type Config struct {
	Level            string   // debug|info|warn|error|fatal (default: info)
	OutputPaths      []string // default: stdout
	ErrorOutputPaths []string // default: OutputPaths
}

// New creates a zap-backed Logger with the provided config.
func New(cfg Config) (Logger, error) {
	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(parseLevel(cfg.Level))
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.EncoderConfig.CallerKey = ""
	zapCfg.DisableCaller = true
	zapCfg.DisableStacktrace = true
	zapCfg.EncoderConfig.StacktraceKey = ""

	if len(cfg.OutputPaths) == 0 {
		zapCfg.OutputPaths = []string{"stdout"}
	} else {
		zapCfg.OutputPaths = cfg.OutputPaths
	}
	if len(cfg.ErrorOutputPaths) == 0 {
		zapCfg.ErrorOutputPaths = zapCfg.OutputPaths
	} else {
		zapCfg.ErrorOutputPaths = cfg.ErrorOutputPaths
	}

	base, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}
	return &zapLogger{base: base, sugar: base.Sugar()}, nil
}

// MustNew is a convenience helper that panics on error.
func MustNew(cfg Config) Logger {
	l, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return l
}

// AsZapSugared returns the underlying zap SugaredLogger when available.
func AsZapSugared(l Logger) (*zap.SugaredLogger, bool) {
	zl, ok := l.(*zapLogger)
	if !ok {
		return nil, false
	}
	return zl.sugar, true
}

type zapLogger struct {
	base  *zap.Logger
	sugar *zap.SugaredLogger
}

func (l *zapLogger) Debugf(template string, args ...any) {
	l.sugar.Debugf(template, args...)
}

func (l *zapLogger) Infof(template string, args ...any) {
	l.sugar.Infof(template, args...)
}

func (l *zapLogger) Warnf(template string, args ...any) {
	l.sugar.Warnf(template, args...)
}

func (l *zapLogger) Errorf(template string, args ...any) {
	l.sugar.Errorf(template, args...)
}

func (l *zapLogger) With(fields ...Field) Logger {
	if len(fields) == 0 {
		return l
	}
	zfs := make([]zap.Field, 0, len(fields))
	for _, f := range fields {
		zfs = append(zfs, zap.Any(f.Key, f.Value))
	}
	nb := l.base.With(zfs...)
	return &zapLogger{base: nb, sugar: nb.Sugar()}
}

func (l *zapLogger) Named(name string) Logger {
	nb := l.base.Named(name)
	return &zapLogger{base: nb, sugar: nb.Sugar()}
}

func (l *zapLogger) Sync() error {
	return l.base.Sync()
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}
