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
)

// InitFlags registers CLI flags and env overrides.
func InitFlags() {
	flag.StringVar(&LogLevel, "log-level", "info", "Server log level")
	flag.IntVar(&Port, "port", 28888, "Server listening port (default: 28888)")
	flag.StringVar(&Namespace, "namespace", "opensandbox", "The Kubernetes namespace to watch for sandbox resources")
	flag.StringVar(&ProviderType, "provider-type", "batchsandbox", "The sandbox provider type (default: batchsandbox)")

	// Parse flags - these will override environment variables if provided
	flag.Parse()
}
