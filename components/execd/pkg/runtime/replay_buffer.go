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

package runtime

import "sync"

const defaultReplayBufSize = 1 << 20 // 1 MiB

// replayBuffer is a bounded circular output buffer that allows reconnecting
// clients to replay missed output from a given byte offset.
type replayBuffer struct {
	mu    sync.Mutex
	buf   []byte // circular storage
	size  int    // capacity
	head  int    // next write position (wraps mod size)
	total int64  // total bytes ever written (monotonic offset)
}

func newReplayBuffer(size int) *replayBuffer {
	return &replayBuffer{buf: make([]byte, size), size: size}
}

// write appends p to the ring buffer, overwriting oldest bytes if full.
func (r *replayBuffer) write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range p {
		r.buf[r.head] = b
		r.head = (r.head + 1) % r.size
		r.total++
	}
}

// readFrom returns all bytes from offset onward (up to buffer capacity).
// Returns (data, nextOffset).
//   - If offset >= total, returns (nil, total) — client is caught up.
//   - If offset is too old (evicted), reads from the oldest available byte.
func (r *replayBuffer) readFrom(offset int64) ([]byte, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	oldest := r.total - int64(r.size)
	if oldest < 0 {
		oldest = 0
	}
	if offset >= r.total {
		return nil, r.total // nothing new
	}
	if offset < oldest {
		offset = oldest // truncated — client missed some output
	}

	n := int(r.total - offset)
	out := make([]byte, n)
	start := int(offset % int64(r.size))
	for i := 0; i < n; i++ {
		out[i] = r.buf[(start+i)%r.size]
	}
	return out, r.total
}
