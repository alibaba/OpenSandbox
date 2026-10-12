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
	"os"
	"reflect"
	"testing"
)

func sampleCounters(values ...int64) map[string]int64 {
	counters := map[string]int64{}
	keys := []string{"oom", "oom_kill", "oom_group_kill"}
	for i, key := range keys {
		if i < len(values) {
			counters[key] = values[i]
		}
	}
	return counters
}

func TestParseOOMEvents(t *testing.T) {
	got := parseOOMEvents([]byte("low 0\nhigh 1\nmax 2\noom 3\noom_kill 4\noom_group_kill 5\nnot-a-counter\nbroken x\n"))
	want := map[string]int64{"low": 0, "high": 1, "max": 2, "oom": 3, "oom_kill": 4, "oom_group_kill": 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("counters = %+v, want %+v", got, want)
	}
	if got := parseOOMEvents([]byte("")); len(got) != 0 {
		t.Fatalf("counters = %+v, want empty", got)
	}
}

func TestParseStat(t *testing.T) {
	stat := []byte("40261 (python3 -m server) S 40254 40261 40261 0 -1 4194560 100 0 0 0 1 0 0 0 20 0 1 0 " +
		"12345 60000000 74000 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0\n")
	got, ok := parseStat(stat)
	if !ok {
		t.Fatal("parseStat failed")
	}
	want := oomProc{Comm: "python3 -m server", Ppid: 40254, RSSBytes: 74000 * int64(os.Getpagesize()), StartTicks: 12345}
	if got != want {
		t.Fatalf("proc = %+v, want %+v", got, want)
	}
}

func TestParseStatMalformed(t *testing.T) {
	for _, data := range []string{"", "40261", "40261 (", "40261 (x)", "1 (a) S 1"} {
		if _, ok := parseStat([]byte(data)); ok {
			t.Fatalf("parseStat(%q) should fail", data)
		}
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
			prev: oomSample{counters: sampleCounters(3, 3), local: 3, haveLocal: true},
			cur:  oomSample{counters: sampleCounters(3, 3), local: 3, haveLocal: true},
			want: oomDelta{haveLocal: true},
		},
		{
			name: "local and descendant kills",
			prev: oomSample{counters: sampleCounters(1, 1), local: 1, haveLocal: true},
			cur:  oomSample{counters: sampleCounters(2, 4), local: 2, haveLocal: true},
			want: oomDelta{total: 3, local: 1, descendant: 2, haveLocal: true},
		},
		{
			name: "descendant only kill",
			prev: oomSample{counters: sampleCounters(2, 2), local: 2, haveLocal: true},
			cur:  oomSample{counters: sampleCounters(3, 3), local: 2, haveLocal: true},
			want: oomDelta{total: 1, descendant: 1, haveLocal: true},
		},
		{
			name: "counter reset counts from zero",
			prev: oomSample{counters: sampleCounters(10, 10), local: 10, haveLocal: true},
			cur:  oomSample{counters: sampleCounters(2, 2), local: 2, haveLocal: true},
			want: oomDelta{total: 2, local: 2, haveLocal: true},
		},
		{
			name: "v1 sample without local counter",
			prev: oomSample{counters: sampleCounters(0, 1)},
			cur:  oomSample{counters: sampleCounters(0, 5)},
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

func TestFormatEventsDelta(t *testing.T) {
	prev := oomSample{counters: sampleCounters(2, 5, 1), haveLocal: true}
	cur := oomSample{counters: sampleCounters(3, 6, 1), haveLocal: true}
	if got, want := formatEventsDelta(prev, cur), "events=[oom=1 oom_kill=1 oom_group_kill=0]"; got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
	if got := formatEventsDelta(oomSample{}, oomSample{counters: sampleCounters(1, 1)}); got != "" {
		t.Fatalf("v1 sample should skip events, got %q", got)
	}
	if got := formatEventsDelta(oomSample{}, oomSample{counters: map[string]int64{}, haveLocal: true}); got != "" {
		t.Fatalf("empty counters should skip events, got %q", got)
	}
}

func TestDiffVictims(t *testing.T) {
	prev := map[int]oomProc{
		10:  {Comm: "python3", Ppid: 1, RSSBytes: 100, StartTicks: 5},
		20:  {Comm: "node", Ppid: 1, RSSBytes: 200, StartTicks: 6},
		30:  {Comm: "bash", Ppid: 10, RSSBytes: 300, StartTicks: 7},
		301: {Comm: "worker", Ppid: 1, RSSBytes: 400, StartTicks: 8},
	}
	cur := map[int]oomProc{
		10:  {Comm: "python3", Ppid: 1, RSSBytes: 100, StartTicks: 5},
		301: {Comm: "worker-renamed", Ppid: 1, RSSBytes: 400, StartTicks: 8},
		40:  {Comm: "new", Ppid: 1, RSSBytes: 500, StartTicks: 9},
	}
	got := diffVictims(prev, cur)
	want := []oomVictim{
		{Pid: 20, Proc: oomProc{Comm: "node", Ppid: 1, RSSBytes: 200, StartTicks: 6}},
		{Pid: 30, Proc: oomProc{Comm: "bash", Ppid: 10, RSSBytes: 300, StartTicks: 7}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("victims = %+v, want %+v", got, want)
	}
}

func TestDiffVictimsEmpty(t *testing.T) {
	if got := diffVictims(nil, nil); got != nil {
		t.Fatalf("victims = %+v, want nil", got)
	}
	prev := map[int]oomProc{1: {Comm: "a"}}
	cur := map[int]oomProc{1: {Comm: "a"}, 2: {Comm: "b"}}
	if got := diffVictims(prev, cur); got != nil {
		t.Fatalf("victims = %+v, want nil", got)
	}
}

func TestVictimAge(t *testing.T) {
	// Boot at t=1000; process started 50s after boot; now is t=3000.
	const boot, start, now = 1000, 50 * clockTicksPerSecond, 3000
	if got := victimAge(boot, start, now); got != 1950 {
		t.Fatalf("age = %d, want 1950", got)
	}
	if got := victimAge(0, start, now); got != -1 {
		t.Fatalf("age without boot time = %d, want -1", got)
	}
	if got := victimAge(boot, 0, now); got != -1 {
		t.Fatalf("age without start ticks = %d, want -1", got)
	}
	if got := victimAge(boot, start, boot); got != -1 {
		t.Fatalf("age before start = %d, want -1", got)
	}
}

func TestFormatVictims(t *testing.T) {
	if got := formatVictims(nil, 0, 0); got != "" {
		t.Fatalf("format = %q, want empty", got)
	}
	victim := oomVictim{Pid: 7, Proc: oomProc{Comm: "python3", Ppid: 1, RSSBytes: 2 << 20, StartTicks: 100}}
	// age = now - boot - startTicks/HZ = 11 - 10 - 1 = 0.
	if got, want := formatVictims([]oomVictim{victim}, 10, 11),
		" victims=[pid=7 comm=python3 ppid=1 rss=2.0MiB age=0s]"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
	// Unknown boot time: age omitted.
	if got, want := formatVictims([]oomVictim{victim}, 0, 11),
		" victims=[pid=7 comm=python3 ppid=1 rss=2.0MiB]"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}

	var many []oomVictim
	for i := 0; i < oomMaxVictims+2; i++ {
		many = append(many, oomVictim{Pid: i + 1, Proc: oomProc{Comm: "p", Ppid: 1}})
	}
	got := formatVictims(many, 0, 0)
	want := " victims=[pid=1 comm=p ppid=1 rss=0.0MiB, pid=2 comm=p ppid=1 rss=0.0MiB, " +
		"pid=3 comm=p ppid=1 rss=0.0MiB, pid=4 comm=p ppid=1 rss=0.0MiB, pid=5 comm=p ppid=1 rss=0.0MiB, +2 more]"
	if got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0.0MiB"},
		{2 << 20, "2.0MiB"},
		{295 * 1 << 20, "295.0MiB"},
		{3 << 30, "3.0GiB"},
		{(3<<30 + 1<<29), "3.5GiB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.n); got != tt.want {
			t.Fatalf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
