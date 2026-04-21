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
	"sort"
	"strings"
	"testing"
)

func TestBuildWatchNamespaces(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string // sorted list of namespaces; nil means cluster-wide (nil map)
		wantErr bool
	}{
		{
			name: "empty string returns cluster-wide",
			raw:  "",
			want: nil,
		},
		{
			name: "whitespace-only returns cluster-wide",
			raw:  "   \t  ",
			want: nil,
		},
		{
			name: "single namespace",
			raw:  "default",
			want: []string{"default"},
		},
		{
			name: "two namespaces",
			raw:  "ns-a,ns-b",
			want: []string{"ns-a", "ns-b"},
		},
		{
			name: "surrounding whitespace is trimmed",
			raw:  "  ns-a  ,   ns-b ",
			want: []string{"ns-a", "ns-b"},
		},
		{
			name: "duplicates are collapsed",
			raw:  "ns-a,ns-a,ns-b",
			want: []string{"ns-a", "ns-b"},
		},
		{
			name:    "single comma rejected",
			raw:     ",",
			wantErr: true,
		},
		{
			name:    "whitespace between commas rejected",
			raw:     " , , ",
			wantErr: true,
		},
		{
			name:    "empty segment in middle rejected",
			raw:     "ns-a,,ns-b",
			wantErr: true,
		},
		{
			name:    "trailing empty segment rejected",
			raw:     "ns-a,",
			wantErr: true,
		},
		{
			name:    "leading empty segment rejected",
			raw:     ",ns-a",
			wantErr: true,
		},
		{
			name:    "uppercase rejected (DNS-1123)",
			raw:     "Default",
			wantErr: true,
		},
		{
			name:    "underscore rejected (DNS-1123)",
			raw:     "bad_ns",
			wantErr: true,
		},
		{
			name:    "overlong name rejected (> 63 chars)",
			raw:     strings.Repeat("a", 64),
			wantErr: true,
		},
		{
			name: "max-length name accepted (63 chars)",
			raw:  strings.Repeat("a", 63),
			want: []string{strings.Repeat("a", 63)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildWatchNamespaces(tc.raw)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("buildWatchNamespaces(%q): expected error, got nil (result=%v)", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildWatchNamespaces(%q): unexpected error: %v", tc.raw, err)
			}

			if tc.want == nil {
				if got != nil {
					t.Fatalf("buildWatchNamespaces(%q): expected nil map (cluster-wide), got %v", tc.raw, got)
				}
				return
			}

			gotKeys := make([]string, 0, len(got))
			for k := range got {
				gotKeys = append(gotKeys, k)
			}
			sort.Strings(gotKeys)

			if len(gotKeys) != len(tc.want) {
				t.Fatalf("buildWatchNamespaces(%q): got %d keys %v, want %d keys %v",
					tc.raw, len(gotKeys), gotKeys, len(tc.want), tc.want)
			}
			for i := range gotKeys {
				if gotKeys[i] != tc.want[i] {
					t.Fatalf("buildWatchNamespaces(%q): key[%d]=%q, want %q (full=%v)",
						tc.raw, i, gotKeys[i], tc.want[i], gotKeys)
				}
			}
		})
	}
}
