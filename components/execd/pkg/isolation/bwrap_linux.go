//go:build linux

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

package isolation

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/alibaba/opensandbox/execd/pkg/log"
)

// bwrapPath is the path to the bwrap binary. It is discovered at startup by
// findBwrap and cached for subsequent use.
var bwrapPath string

// seccompBPF holds pre-generated seccomp BPF bytecode, initialised once at
// startup by generateSeccompDenyBPF.
var seccompBPF []byte

// findBwrap locates the bwrap binary. Priority order:
//
//  1. $PATH lookup                 — respect user-installed bwrap
//  2. /opt/opensandbox/bin/bwrap   — injected by init container alongside execd
//  3. /usr/bin/bwrap               — system package (Alpine apk)
//  4. /usr/local/bin/bwrap         — manual install
func findBwrap() string {
	// First: respect whatever the user has in $PATH.
	if path, err := exec.LookPath("bwrap"); err == nil {
		return path
	}
	// Fall back to known locations.
	for _, p := range []string{
		"/opt/opensandbox/bin/bwrap",
		"/usr/bin/bwrap",
		"/usr/local/bin/bwrap",
	} {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	return ""
}

// bwrapImpl is the Linux bwrap Isolator.
type bwrapImpl struct{}

// NewBwrap returns a bwrap Isolator for Linux, configured by cfg.
func NewBwrap(cfg Config) Isolator {
	bwrapPath = findBwrap()

	// Pre-generate seccomp BPF once at startup.
	if bpf, err := generateSeccompDenyBPF(cfg.Seccomp); err != nil {
		log.Warning("seccomp: failed to generate BPF: %v", err)
	} else {
		seccompBPF = bpf
	}

	return &bwrapImpl{}
}

func (b *bwrapImpl) Name() string { return "bwrap" }

func (b *bwrapImpl) Available() bool {
	if bwrapPath == "" {
		bwrapPath = findBwrap()
	}
	return bwrapPath != ""
}

func (b *bwrapImpl) Capabilities() Capabilities {
	if bwrapPath == "" {
		bwrapPath = findBwrap()
	}

	version, err := probeBwrapVersion()
	if err != nil {
		version = ""
	}

	return Capabilities{
		Available:              bwrapPath != "",
		Isolator:               "bwrap",
		Version:                version,
		Profiles:               []Profile{ProfileStrict, ProfileBalanced},
		ShareNetOverridable:    true,
		CommitSupported:        false, // Phase 2
		DiffSupported:          false, // Phase 2
		PersistAvailable:       false, // Phase 2
		PersistMaxBytesDefault: 2 * 1024 * 1024 * 1024,
		PersistMaxBytesLimit:   8 * 1024 * 1024 * 1024,
		PersistRetainDefault:   3600,
	}
}

func (b *bwrapImpl) Wrap(cmd *exec.Cmd, opts WrapOptions) error {
	if bwrapPath == "" {
		bwrapPath = findBwrap()
	}
	if bwrapPath == "" {
		return fmt.Errorf("bwrap: binary not found")
	}

	// Wire up seccomp BPF via memfd, if available.
	var seccompFd string
	if len(seccompBPF) > 0 {
		fd, err := createMemfdWithData(seccompBPF)
		if err != nil {
			return fmt.Errorf("bwrap: seccomp memfd: %w", err)
		}
		// ExtraFiles are assigned fds starting at 3 in the child process.
		seccompFd = strconv.Itoa(3 + len(cmd.ExtraFiles))
		cmd.ExtraFiles = append(cmd.ExtraFiles, os.NewFile(uintptr(fd), "seccomp"))
	}

	argv, err := buildArgv(opts, seccompFd)
	if err != nil {
		return fmt.Errorf("bwrap: %w", err)
	}

	wrapWithArgv(cmd, bwrapPath, argv)
	return nil
}

// createMemfdWithData creates an anonymous memfd, writes data to it, and
// seeks back to the beginning. The returned fd is ready to be passed to bwrap
// via ExtraFiles.
func createMemfdWithData(data []byte) (int, error) {
	fd, err := unix.MemfdCreate("seccomp", 0)
	if err != nil {
		return -1, fmt.Errorf("memfd_create: %w", err)
	}
	// Write data and seek back to 0 so bwrap reads from the start.
	if _, err := unix.Write(fd, data); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("write seccomp BPF: %w", err)
	}
	if _, err := unix.Seek(fd, 0, 0); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("seek seccomp BPF: %w", err)
	}
	return fd, nil
}

// Ensure bwrapImpl satisfies Isolator.
var _ Isolator = (*bwrapImpl)(nil)
