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

package policy

import (
	"fmt"
	"io/fs"
)

// FileMutationState describes the authoritative policy path, not temporary files.
// A caller must not infer FileUnchanged merely because rename returned an error.
type FileMutationState uint8

const (
	// FileUnchanged means no rename or unlink of the authoritative path was attempted.
	FileUnchanged FileMutationState = iota
	// FileCommitted means the operation and its parent-directory fsync completed.
	FileCommitted
	// FileUnknown means publication was attempted, or its durability/close failed.
	FileUnknown
)

// FileMutationError identifies an I/O phase without including policy contents.
type FileMutationError struct {
	Phase string
	Err   error
}

func (e *FileMutationError) Error() string {
	return fmt.Sprintf("atomic policy file %s: %v", e.Phase, e.Err)
}
func (e *FileMutationError) Unwrap() error { return e.Err }

func atomicFileError(phase string, err error) error {
	return &FileMutationError{Phase: phase, Err: err}
}

// PolicyFileSnapshot is an opaque, exact image of one policy path. It preserves
// absence, bytes, permission bits and ownership plus SELinux comparison state;
// it is not a reserialized policy or a copy of integrity attributes.
type PolicyFileSnapshot struct {
	path     string
	exists   bool
	data     []byte
	mode     fs.FileMode
	uid      int
	gid      int
	security policyFileSecurity
}

// policyFileSecurity records only comparison/presence information. Integrity
// attribute payloads are never retained or copied onto replacement contents.
type policyFileSecurity struct {
	selinux  string
	ima, evm bool
}
