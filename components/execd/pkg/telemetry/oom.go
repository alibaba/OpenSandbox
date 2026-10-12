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
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/log"
	"github.com/alibaba/opensandbox/internal/safego"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// oomPollInterval balances detection latency against cgroupfs polling
	// cost; kills within one interval collapse into a single counter diff.
	oomPollInterval = 5 * time.Second
	// oomMaxVictims caps victim pid/comm pairs in one log line.
	oomMaxVictims = 5
	// oomMaxSnapshotPids caps the per-poll /proc scan used for attribution.
	oomMaxSnapshotPids = 2048
)

// StartOOMWatch launches the sandbox cgroup OOM kill watcher. Every detected
// kill increments execd.system.oom.kills and emits a troubleshooting log
// line. The watcher runs independently of OTLP export so OOM logs work even
// when metrics are disabled. The returned function stops the watcher.
func StartOOMWatch(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	safego.Go(func() { runOOMWatch(ctx) })
	return cancel
}

// oomSample is one read of the cgroup OOM counters. total counts kills in the
// whole watched cgroup subtree; local counts kills of processes belonging to
// the watched cgroup itself, without descendants (cgroup v2 only).
type oomSample struct {
	total     int64
	local     int64
	haveLocal bool
}

// oomDelta is the change between two samples.
type oomDelta struct {
	total      int64
	local      int64
	descendant int64
	haveLocal  bool
}

// oomEventSource identifies where a detection came from, for logging.
type oomEventSource struct {
	Mode       string // e.g. "cgroup-v2"
	CgroupPath string // sandbox cgroup path as seen inside the container
}

// oomVictim is a best-effort attribution of a killed process: a pid that
// disappeared from /proc between the polls surrounding an OOM event.
type oomVictim struct {
	Pid  int
	Comm string
}

// diffOOMSample computes the delta between two counter samples. A shrinking
// counter is treated as a reset and counted from zero.
func diffOOMSample(prev, cur oomSample) oomDelta {
	delta := oomDelta{haveLocal: cur.haveLocal}
	delta.total = monotonicDelta(prev.total, cur.total)
	if cur.haveLocal {
		delta.local = monotonicDelta(prev.local, cur.local)
		delta.descendant = delta.total - delta.local
	}
	return delta
}

func monotonicDelta(prev, cur int64) int64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

// oomKillFromEvents extracts the oom_kill counter from cgroup memory event
// content ("<key> <value>" lines: memory.events, memory.events.local, or
// memory.oom_control).
func oomKillFromEvents(data []byte) (int64, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "oom_kill" {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

// diffVictims returns processes present in prev but gone from cur, ordered
// by pid.
func diffVictims(prev, cur map[int]string) []oomVictim {
	var victims []oomVictim
	for pid, comm := range prev {
		if _, alive := cur[pid]; alive {
			continue
		}
		victims = append(victims, oomVictim{Pid: pid, Comm: comm})
	}
	sort.Slice(victims, func(i, j int) bool { return victims[i].Pid < victims[j].Pid })
	return victims
}

// emitOOMKill reports one detection: metric increments plus a log line.
// Metric attributes stay low-cardinality (level only); pid/comm attribution
// lives in the log because pids churn and comms are user-controlled.
func emitOOMKill(ctx context.Context, src oomEventSource, delta oomDelta, victims []oomVictim) {
	if oomKills != nil && delta.total > 0 {
		if delta.haveLocal {
			if delta.local > 0 {
				oomKills.Add(ctx, delta.local, metric.WithAttributes(oomLevelAttrs("local")...))
			}
			if delta.descendant > 0 {
				oomKills.Add(ctx, delta.descendant, metric.WithAttributes(oomLevelAttrs("descendant")...))
			}
		} else {
			oomKills.Add(ctx, delta.total, metric.WithAttributes(sharedAttrs()...))
		}
	}

	summary := formatVictims(victims)
	if delta.haveLocal {
		log.Warn("oom: detected %d OOM kill(s) (local=%d, descendant=%d, source=%s, cgroup=%s)%s",
			delta.total, delta.local, delta.descendant, src.Mode, src.CgroupPath, summary)
	} else {
		log.Warn("oom: detected %d OOM kill(s) (source=%s, cgroup=%s)%s",
			delta.total, src.Mode, src.CgroupPath, summary)
	}
}

func oomLevelAttrs(level string) []attribute.KeyValue {
	attrs := append([]attribute.KeyValue{}, sharedAttrs()...)
	return append(attrs, attribute.String("level", level))
}

// formatVictims renders victims for the log line, capped at oomMaxVictims.
func formatVictims(victims []oomVictim) string {
	if len(victims) == 0 {
		return ""
	}
	shown := victims
	extra := 0
	if len(victims) > oomMaxVictims {
		shown = victims[:oomMaxVictims]
		extra = len(victims) - oomMaxVictims
	}
	parts := make([]string, 0, len(shown)+1)
	for _, victim := range shown {
		parts = append(parts, fmt.Sprintf("pid=%d comm=%s", victim.Pid, victim.Comm))
	}
	if extra > 0 {
		parts = append(parts, fmt.Sprintf("+%d more", extra))
	}
	return " victims=[" + strings.Join(parts, ", ") + "]"
}
