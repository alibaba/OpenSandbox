// Copyright 2026 The OpenSandbox Authors
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

package telemetry

import (
	"reflect"
	"testing"
)

func TestOOMKillFromEvents(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		want      int64
		wantFound bool
	}{
		{
			name:      "memory.events with multiple counters",
			data:      "oom 1\noom_kill 3\npgmajfault 42\n",
			want:      3,
			wantFound: true,
		},
		{
			name:      "first oom_kill wins on duplicates",
			data:      "oom_kill 7\noom_kill 8\n",
			want:      7,
			wantFound: true,
		},
		{
			name:      "no trailing newline",
			data:      "oom_kill 0",
			want:      0,
			wantFound: true,
		},
		{
			name:      "missing counter",
			data:      "oom 1\npgmajfault 2\n",
			wantFound: false,
		},
		{
			name:      "malformed value",
			data:      "oom_kill notanumber\n",
			wantFound: false,
		},
		{
			name:      "empty content",
			data:      "",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := oomKillFromEvents([]byte(tt.data))
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if found && got != tt.want {
				t.Fatalf("value = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDiffOOMSample(t *testing.T) {
	tests := []struct {
		name string
		prev oomSample
		cur  oomSample
		want oomDelta
	}{
		{
			name: "no change",
			prev: oomSample{total: 3, local: 3, haveLocal: true},
			cur:  oomSample{total: 3, local: 3, haveLocal: true},
			want: oomDelta{haveLocal: true},
		},
		{
			name: "local and descendant kills",
			prev: oomSample{total: 1, local: 1, haveLocal: true},
			cur:  oomSample{total: 4, local: 2, haveLocal: true},
			want: oomDelta{total: 3, local: 1, descendant: 2, haveLocal: true},
		},
		{
			name: "descendant only kill",
			prev: oomSample{total: 2, local: 2, haveLocal: true},
			cur:  oomSample{total: 3, local: 2, haveLocal: true},
			want: oomDelta{total: 1, descendant: 1, haveLocal: true},
		},
		{
			name: "counter reset counts from zero",
			prev: oomSample{total: 10, local: 10, haveLocal: true},
			cur:  oomSample{total: 2, local: 2, haveLocal: true},
			want: oomDelta{total: 2, local: 2, haveLocal: true},
		},
		{
			name: "v1 sample without local counter",
			prev: oomSample{total: 1},
			cur:  oomSample{total: 5},
			want: oomDelta{total: 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffOOMSample(tt.prev, tt.cur)
			if got != tt.want {
				t.Fatalf("delta = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDiffVictims(t *testing.T) {
	prev := map[int]string{
		10:  "python3",
		20:  "node",
		30:  "bash",
		301: "worker",
	}
	cur := map[int]string{
		10:  "python3",
		301: "worker-renamed",
		40:  "new",
	}
	got := diffVictims(prev, cur)
	want := []oomVictim{
		{Pid: 20, Comm: "node"},
		{Pid: 30, Comm: "bash"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("victims = %+v, want %+v", got, want)
	}
}

func TestDiffVictimsEmpty(t *testing.T) {
	if got := diffVictims(nil, nil); got != nil {
		t.Fatalf("victims = %+v, want nil", got)
	}
	prev := map[int]string{1: "a"}
	cur := map[int]string{1: "a", 2: "b"}
	if got := diffVictims(prev, cur); got != nil {
		t.Fatalf("victims = %+v, want nil", got)
	}
}

func TestFormatVictims(t *testing.T) {
	if got := formatVictims(nil); got != "" {
		t.Fatalf("format = %q, want empty", got)
	}
	victims := []oomVictim{{Pid: 7, Comm: "python3"}}
	if got, want := formatVictims(victims), " victims=[pid=7 comm=python3]"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
	var many []oomVictim
	for i := 0; i < oomMaxVictims+2; i++ {
		many = append(many, oomVictim{Pid: i + 1, Comm: "p"})
	}
	got := formatVictims(many)
	want := " victims=[pid=1 comm=p, pid=2 comm=p, pid=3 comm=p, pid=4 comm=p, pid=5 comm=p, +2 more]"
	if got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
}
