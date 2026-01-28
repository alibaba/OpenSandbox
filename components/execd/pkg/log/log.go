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

package log

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const logFileEnvKey = "EXECD_LOG_FILE"

var (
	atomicLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	base        *zap.Logger
	sugar       *zap.SugaredLogger
)

func init() {
	cfg := zap.NewProductionConfig()
	cfg.Level = atomicLevel

	logFile := os.Getenv(logFileEnvKey)
	if logFile != "" {
		cfg.OutputPaths = []string{logFile}
		cfg.ErrorOutputPaths = []string{logFile}
	} else {
		// outputs log to stdout pipe by default
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stdout"}
	}

	logger, err := cfg.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to init logger: %v", err))
	}
	base = logger
	sugar = base.Sugar()
}

// SetLevel maps legacy Beego log levels to zap levels.
// 0/1/2 => Fatal, 3 => Error, 4 => Warn, 5/6 => Info, 7+ => Debug.
func SetLevel(level int) {
	atomicLevel.SetLevel(mapLevel(level))
}

func mapLevel(level int) zapcore.Level {
	switch {
	case level <= 2:
		return zapcore.FatalLevel
	case level == 3:
		return zapcore.ErrorLevel
	case level == 4:
		return zapcore.WarnLevel
	case level == 5 || level == 6:
		return zapcore.InfoLevel
	default:
		return zapcore.DebugLevel
	}
}

func Sync() {
	_ = base.Sync()
}

func Debug(format string, args ...any) {
	sugar.Debugf(format, args...)
}

func Info(format string, args ...any) {
	sugar.Infof(format, args...)
}

func Warn(format string, args ...any) {
	sugar.Warnf(format, args...)
}

// Warning is an alias to Warn for compatibility.
func Warning(format string, args ...any) {
	Warn(format, args...)
}

func Error(format string, args ...any) {
	sugar.Errorf(format, args...)
}
