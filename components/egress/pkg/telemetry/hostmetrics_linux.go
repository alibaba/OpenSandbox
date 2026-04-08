// Copyright 2026 Alibaba Group Holding Ltd.
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

//go:build linux

package telemetry

import (
	"bufio"
	"os"
	"sync"
)

func systemMemoryUsedBytes() int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return meminfoUsedBytesFromContent(b)
}

var cpuJiffyPrev struct {
	sync.Mutex
	total, idle uint64
	hasPrev     bool
}

// cpuUtilizationRatio returns the fraction of CPU time non-idle since the previous observation
// (0–1). The first observation after process start returns 0 (no delta yet).
func cpuUtilizationRatio() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0
	}
	total, idle, ok := procStatCPUJiffies(sc.Text())
	if !ok {
		return 0
	}

	cpuJiffyPrev.Lock()
	defer cpuJiffyPrev.Unlock()
	if !cpuJiffyPrev.hasPrev {
		cpuJiffyPrev.total = total
		cpuJiffyPrev.idle = idle
		cpuJiffyPrev.hasPrev = true
		return 0
	}
	dt := total - cpuJiffyPrev.total
	di := idle - cpuJiffyPrev.idle
	cpuJiffyPrev.total = total
	cpuJiffyPrev.idle = idle
	if dt == 0 || dt < di {
		return 0
	}
	return float64(dt-di) / float64(dt)
}
