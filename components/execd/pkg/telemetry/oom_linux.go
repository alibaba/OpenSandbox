//go:build linux

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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/log"
)

const (
	procCgroupFile     = "/proc/self/cgroup"
	procDir            = "/proc"
	cgroupV2DefaultDir = "/sys/fs/cgroup"
	cgroupV1MemoryDir  = "/sys/fs/cgroup/memory"
)

// oomSource is the cgroupfs location of the sandbox OOM counters. execd sits
// in the sandbox's own cgroup (often also its own cgroup namespace), so its
// /proc/self/cgroup path resolved under the cgroup mount scopes the whole
// sandbox subtree: kills of user processes and of nested cgroups (e.g.
// isolated sessions) all show up in the counters read here.
type oomSource struct {
	mode       string
	cgroupPath string
	eventsPath string // memory.events (v2) or memory.oom_control (v1)
	localPath  string // memory.events.local (v2 only, empty otherwise)
}

// runOOMWatch polls the sandbox cgroup OOM counters until ctx is done.
func runOOMWatch(ctx context.Context) {
	source, err := newOOMSource()
	if err != nil {
		log.Warn("oom: watch disabled: %v", err)
		return
	}
	watchOOMSource(ctx, source, oomPollInterval)
}

// watchOOMSource polls source until ctx is done. The first sample only seeds
// the baseline; each later increment is reported as a kill event.
func watchOOMSource(ctx context.Context, source *oomSource, interval time.Duration) {
	log.Info("oom: watching for OOM kills (source=%s, cgroup=%s, interval=%s)",
		source.mode, source.cgroupPath, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	havePrev := false
	var prev oomSample
	var prevProcs map[int]string
	readErrReported := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		cur, err := source.sample()
		if err != nil {
			// Report once: repeated failures (e.g. cgroupfs gone during
			// teardown) would otherwise spam the log every interval.
			if !readErrReported {
				readErrReported = true
				log.Warn("oom: reading %s failed, retrying: %v", source.eventsPath, err)
			}
			continue
		}
		readErrReported = false

		curProcs := snapshotProcesses()

		if havePrev {
			if delta := diffOOMSample(prev, cur); delta.total > 0 {
				emitOOMKill(ctx, source.eventSource(), delta, diffVictims(prevProcs, curProcs))
			}
		}
		prev, prevProcs, havePrev = cur, curProcs, true
	}
}

func newOOMSource() (*oomSource, error) {
	if source, err := newCgroupV2Source(); err == nil {
		return source, nil
	}
	if source, err := newCgroupV1Source(); err == nil {
		return source, nil
	}
	return nil, errors.New("no cgroup v2 or v1 memory events interface visible")
}

func newCgroupV2Source() (*oomSource, error) {
	cgroupPath, err := cgroupV2RelPath()
	if err != nil {
		return nil, err
	}
	mount, err := cgroupV2Mount()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(mount, filepath.FromSlash(cgroupPath))
	eventsPath := filepath.Join(dir, "memory.events")
	if _, err := os.Stat(eventsPath); err != nil {
		return nil, fmt.Errorf("stat %s: %w", eventsPath, err)
	}
	source := &oomSource{
		mode:       "cgroup-v2",
		cgroupPath: cgroupPath,
		eventsPath: eventsPath,
	}
	// memory.events.local separates kills of processes in this cgroup from
	// kills propagated from descendants; absent on very old kernels.
	localPath := filepath.Join(dir, "memory.events.local")
	if _, err := os.Stat(localPath); err == nil {
		source.localPath = localPath
	}
	return source, nil
}

func newCgroupV1Source() (*oomSource, error) {
	cgroupPath, err := cgroupV1MemoryRelPath()
	if err != nil {
		return nil, err
	}
	mount, err := cgroupV1MemoryMount()
	if err != nil {
		return nil, err
	}
	// v1 counters are per-cgroup: this is best-effort and does not cover
	// descendant cgroups the way the v2 hierarchical counters do.
	oomControl := filepath.Join(mount, filepath.FromSlash(cgroupPath), "memory.oom_control")
	if _, err := os.Stat(oomControl); err != nil {
		return nil, fmt.Errorf("stat %s: %w", oomControl, err)
	}
	return &oomSource{
		mode:       "cgroup-v1",
		cgroupPath: cgroupPath,
		eventsPath: oomControl,
	}, nil
}

func (s *oomSource) eventSource() oomEventSource {
	return oomEventSource{Mode: s.mode, CgroupPath: s.cgroupPath}
}

func (s *oomSource) sample() (oomSample, error) {
	data, err := os.ReadFile(s.eventsPath)
	if err != nil {
		return oomSample{}, err
	}
	total, ok := oomKillFromEvents(data)
	if !ok {
		return oomSample{}, fmt.Errorf("no oom_kill counter in %s", s.eventsPath)
	}
	sample := oomSample{total: total}
	if s.localPath != "" {
		data, err := os.ReadFile(s.localPath)
		if err != nil {
			return oomSample{}, err
		}
		local, ok := oomKillFromEvents(data)
		if !ok {
			return oomSample{}, fmt.Errorf("no oom_kill counter in %s", s.localPath)
		}
		sample.local = local
		sample.haveLocal = true
	}
	return sample, nil
}

// cgroupV2RelPath returns the sandbox's cgroup v2 path relative to the
// hierarchy root, from the "0::<path>" line of /proc/self/cgroup.
func cgroupV2RelPath() (string, error) {
	data, err := os.ReadFile(procCgroupFile)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if path, ok := strings.CutPrefix(line, "0::"); ok && path != "" {
			return path, nil
		}
	}
	return "", errors.New("no cgroup v2 entry in /proc/self/cgroup")
}

// cgroupV2Mount locates the cgroup v2 filesystem mount.
func cgroupV2Mount() (string, error) {
	if _, err := os.Stat(filepath.Join(cgroupV2DefaultDir, "cgroup.controllers")); err == nil {
		return cgroupV2DefaultDir, nil
	}
	return scanMounts(func(mountPoint, fstype string, options []string) bool {
		return fstype == "cgroup2"
	})
}

// cgroupV1MemoryRelPath returns the sandbox's path in the v1 memory
// hierarchy, from /proc/self/cgroup.
func cgroupV1MemoryRelPath() (string, error) {
	data, err := os.ReadFile(procCgroupFile)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		for _, controller := range strings.Split(parts[1], ",") {
			if controller == "memory" && parts[2] != "" {
				return parts[2], nil
			}
		}
	}
	return "", errors.New("no memory hierarchy entry in /proc/self/cgroup")
}

// cgroupV1MemoryMount locates the v1 memory controller mount.
func cgroupV1MemoryMount() (string, error) {
	if _, err := os.Stat(cgroupV1MemoryDir); err == nil {
		return cgroupV1MemoryDir, nil
	}
	return scanMounts(func(mountPoint, fstype string, options []string) bool {
		if fstype != "cgroup" {
			return false
		}
		for _, option := range options {
			if option == "memory" {
				return true
			}
		}
		return false
	})
}

func scanMounts(match func(mountPoint, fstype string, options []string) bool) (string, error) {
	data, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if match(fields[1], fields[2], fields[3:]) {
			return fields[1], nil
		}
	}
	return "", errors.New("no matching mount in /proc/self/mounts")
}

// snapshotProcesses captures the pid->comm map visible to execd (the sandbox
// pid namespace) for victim attribution. Best effort: a partial or empty map
// degrades attribution, never the OOM detection itself.
func snapshotProcesses() map[int]string {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil
	}
	procs := make(map[int]string, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(procDir, entry.Name(), "comm"))
		if err != nil {
			// Exited between readdir and read; it stays in the previous
			// snapshot and may be reported as a victim with its old comm.
			continue
		}
		procs[pid] = strings.TrimSuffix(string(comm), "\n")
		if len(procs) >= oomMaxSnapshotPids {
			break
		}
	}
	return procs
}
