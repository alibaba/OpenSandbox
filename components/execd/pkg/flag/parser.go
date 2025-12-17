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

package flag

import (
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/beego/beego/v2/core/logs"
)

const (
	jupyterHostEnv  = "JUPYTER_HOST"
	jupyterTokenEnv = "JUPYTER_TOKEN"
)

// InitFlags registers CLI flags and env overrides.
func InitFlags() {
	// Set default values
	ServerPort = 44772
	ServerLogLevel = 6
	ServerAccessToken = ""
	ApiGracefulShutdownTimeout = time.Second * 3

	// First, set default values from environment variables
	if jupyterFromEnv := os.Getenv(jupyterHostEnv); jupyterFromEnv != "" {
		if !strings.HasPrefix(jupyterFromEnv, "http://") && !strings.HasPrefix(jupyterFromEnv, "https://") {
			log.Panic("Invalid JUPYTER_HOST format: must start with http:// or https://")
		}
		JupyterServerHost = jupyterFromEnv
	}

	if jupyterTokenFromEnv := os.Getenv(jupyterTokenEnv); jupyterTokenFromEnv != "" {
		JupyterServerToken = jupyterTokenFromEnv
	}

	// Then define flags with current values as defaults
	flag.StringVar(&JupyterServerHost, "jupyter-host", JupyterServerHost, "Jupyter server host address (e.g., http://localhost, http://192.168.1.100)")
	flag.StringVar(&JupyterServerToken, "jupyter-token", JupyterServerToken, "Jupyter server authentication token")
	flag.IntVar(&ServerPort, "port", ServerPort, "Server listening port (default: 44772)")
	flag.IntVar(&ServerLogLevel, "log-level", ServerLogLevel, "Server log level (0=LevelEmergency, 1=LevelAlert, 2=LevelCritical, 3=LevelError, 4=LevelWarning, 5=LevelNotice, 6=LevelInformational, 7=LevelDebug, default: 6)")
	flag.StringVar(&ServerAccessToken, "access-token", ServerAccessToken, "Server access token for API authentication")
	flag.DurationVar(&ApiGracefulShutdownTimeout, "graceful-shutdown-timeout", ApiGracefulShutdownTimeout, "API graceful shutdown timeout duration (default: 3s)")

	// Parse flags - these will override environment variables if provided
	flag.Parse()

	// Log final values
	logs.Info("Jupyter server host is: %s", JupyterServerHost)
	logs.Info("Jupyter server token is: %s", JupyterServerToken)
}
