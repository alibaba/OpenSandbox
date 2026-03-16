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

import (
	"bytes"
	"sync"
	"testing"
)

func TestReplayBuffer_WriteAndRead(t *testing.T) {
	rb := newReplayBuffer(64)
	data := []byte("hello world\n")
	rb.write(data)

	got, next := rb.readFrom(0)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
	if next != int64(len(data)) {
		t.Fatalf("expected next=%d, got %d", len(data), next)
	}
}

func TestReplayBuffer_CircularEviction(t *testing.T) {
	size := 16
	rb := newReplayBuffer(size)

	// Write 20 bytes — 4 bytes will be evicted.
	first := []byte("AAAA")  // will be evicted
	second := []byte("BBBBBBBBBBBBBBBB") // 16 bytes fills the buffer
	rb.write(first)
	rb.write(second)

	// total == 20, oldest == 4
	got, next := rb.readFrom(0) // offset 0 is too old, should be clamped to oldest
	if next != 20 {
		t.Fatalf("expected next=20, got %d", next)
	}
	// Should get exactly 16 bytes (the second write, which overwrote first)
	if len(got) != size {
		t.Fatalf("expected %d bytes, got %d", size, len(got))
	}
	if !bytes.Equal(got, second) {
		t.Fatalf("expected %q, got %q", second, got)
	}
}

func TestReplayBuffer_OffsetCaughtUp(t *testing.T) {
	rb := newReplayBuffer(64)
	rb.write([]byte("some output\n"))

	total := rb.total
	got, next := rb.readFrom(total)
	if got != nil {
		t.Fatalf("expected nil for caught-up offset, got %q", got)
	}
	if next != total {
		t.Fatalf("expected next=%d, got %d", total, next)
	}
}

func TestReplayBuffer_LargeGap(t *testing.T) {
	size := 8
	rb := newReplayBuffer(size)

	// Write 16 bytes total — first 8 are evicted.
	rb.write([]byte("12345678")) // bytes 0-7
	rb.write([]byte("ABCDEFGH")) // bytes 8-15

	// Request from offset 0 (evicted) — should return from oldest available (offset 8).
	got, next := rb.readFrom(0)
	if next != 16 {
		t.Fatalf("expected next=16, got %d", next)
	}
	if !bytes.Equal(got, []byte("ABCDEFGH")) {
		t.Fatalf("expected oldest available data, got %q", got)
	}
}

func TestReplayBuffer_Concurrent(t *testing.T) {
	rb := newReplayBuffer(1024)
	chunk := []byte("x")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rb.write(chunk)
			}
		}()
	}

	// Concurrent reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		var off int64
		for i := 0; i < 50; i++ {
			_, off = rb.readFrom(off)
		}
	}()

	wg.Wait()

	if rb.total != 1000 {
		t.Fatalf("expected total=1000, got %d", rb.total)
	}
}
