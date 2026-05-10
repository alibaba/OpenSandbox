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
	"io"
	"testing"
	"time"
)

func TestControllerOptionsAcceptLegacyManifestFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want func(*testing.T, *controllerOptions)
	}{
		{
			name: "helm controller v0.1.0",
			args: []string{
				"--leader-elect",
				"--health-probe-bind-address=:8081",
				"--v=2",
			},
			want: func(t *testing.T, opts *controllerOptions) {
				t.Helper()
				if opts.legacyKlogVerbosity != "2" {
					t.Fatalf("legacy verbosity = %q, want 2", opts.legacyKlogVerbosity)
				}
			},
		},
		{
			name: "server chart with kube client rate limits",
			args: []string{
				"--leader-elect",
				"--health-probe-bind-address=:8081",
				"--zap-log-level=debug",
				"--kube-client-qps=250",
				"--kube-client-burst=500",
				"--image-committer-image=registry.example.com/image-committer:v1",
				"--commit-job-timeout=15m",
				"--snapshot-registry=registry.example.com/snapshots",
				"--snapshot-registry-insecure=true",
				"--snapshot-push-secret=push-secret",
				"--resume-pull-secret=pull-secret",
			},
			want: func(t *testing.T, opts *controllerOptions) {
				t.Helper()
				if !opts.enableLeaderElection {
					t.Fatalf("enableLeaderElection = false, want true")
				}
				if opts.kubeClientQPS != 250 {
					t.Fatalf("kubeClientQPS = %v, want 250", opts.kubeClientQPS)
				}
				if opts.kubeClientBurst != 500 {
					t.Fatalf("kubeClientBurst = %v, want 500", opts.kubeClientBurst)
				}
				if opts.imageCommitterImage != "registry.example.com/image-committer:v1" {
					t.Fatalf("imageCommitterImage = %q, want registry.example.com/image-committer:v1", opts.imageCommitterImage)
				}
				if opts.commitJobTimeout != 15*time.Minute {
					t.Fatalf("commitJobTimeout = %s, want 15m", opts.commitJobTimeout)
				}
				if opts.snapshotRegistry != "registry.example.com/snapshots" {
					t.Fatalf("snapshotRegistry = %q, want registry.example.com/snapshots", opts.snapshotRegistry)
				}
				if !opts.snapshotRegistryInsecure {
					t.Fatalf("snapshotRegistryInsecure = false, want true")
				}
				if opts.snapshotPushSecret != "push-secret" {
					t.Fatalf("snapshotPushSecret = %q, want push-secret", opts.snapshotPushSecret)
				}
				if opts.resumePullSecret != "pull-secret" {
					t.Fatalf("resumePullSecret = %q, want pull-secret", opts.resumePullSecret)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("controller", flag.ContinueOnError)
			fs.SetOutput(io.Discard)

			opts := &controllerOptions{}
			opts.bindFlags(fs)

			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			tt.want(t, opts)
		})
	}
}
