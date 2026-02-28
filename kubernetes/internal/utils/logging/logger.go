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

package logging

import (
	"os"

	"github.com/go-logr/logr"
	zap2 "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Options contains configuration for the logger
type Options struct {
	// Development configures the logger to use a development config
	Development bool
	// EnableFileOutput enables output to file
	EnableFileOutput bool
	// LogFilePath is the path to the log file
	LogFilePath string
	// MaxSize is the maximum size in megabytes of the log file before it gets rotated
	MaxSize int
	// MaxBackups is the maximum number of old log files to retain
	MaxBackups int
	// MaxAge is the maximum number of days to retain old log files
	MaxAge int
	// Compress determines if the rotated log files should be compressed using gzip
	Compress bool
	// ZapOptions are additional zap options
	ZapOptions zap.Options
}

// DefaultOptions returns default logger options
func DefaultOptions() Options {
	return Options{
		Development:      false,
		EnableFileOutput: false,
		LogFilePath:      "/var/log/sandbox-controller/controller.log",
		MaxSize:          100,  // 100MB
		MaxBackups:       10,   // keep 10 old log files
		MaxAge:           30,   // keep log files for 30 days
		Compress:         true, // compress rotated files
		ZapOptions: zap.Options{
			Development: false,
		},
	}
}

// NewLoggerWithZapOptions creates a logger using controller-runtime's zap options
// and adds file output support
func NewLoggerWithZapOptions(opts Options) logr.Logger {
	// Add AddCaller option to include file and line number in logs
	if opts.ZapOptions.ZapOpts == nil {
		opts.ZapOptions.ZapOpts = []zap2.Option{}
	}
	opts.ZapOptions.ZapOpts = append(opts.ZapOptions.ZapOpts, zap2.AddCaller())

	// If file output is not enabled, use the default zap logger
	if !opts.EnableFileOutput {
		return zap.New(zap.UseFlagOptions(&opts.ZapOptions))
	}

	// Create file writer with rotation
	fileWriter := &lumberjack.Logger{
		Filename:   opts.LogFilePath,
		MaxSize:    opts.MaxSize,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAge,
		Compress:   opts.Compress,
		LocalTime:  true,
	}

	// Create multi-writer that writes to both stdout and file
	multiWriter := zapcore.NewMultiWriteSyncer(
		zapcore.AddSync(os.Stdout),
		zapcore.AddSync(fileWriter),
	)

	// Create logger with multi-writer
	return zap.New(
		zap.UseFlagOptions(&opts.ZapOptions),
		zap.WriteTo(multiWriter),
	)
}
