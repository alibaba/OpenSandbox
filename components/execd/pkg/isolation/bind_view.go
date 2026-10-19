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

package isolation

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/opensandbox/execd/pkg/vfs"
)

// BindView serves the files-API view of one bind mount. Requests arrive
// under the bind's in-session destination; paths are rewritten dest →
// relative and forwarded to a MergedView rooted at the host source
// directory. A bind is a 1:1 mapping to a real host directory (validated
// against the allowed_writable allowlist with symlink resolution at
// creation), so reads and writes are live in both directions — the
// overlay limitation (host writes invisible to a running bwrap overlayfs)
// does not apply. Read-only binds reject writes through the MergedView's
// existing ro semantics (#2297).
type BindView struct {
	dest string
	view *MergedView
}

var _ vfs.FS = (*BindView)(nil)

// NewBindView creates the files-API view for one bind mount. dest is the
// mount destination inside the namespace; source is the host directory the
// bind maps to.
func NewBindView(dest, source string, readOnly bool, uid, gid uint32) *BindView {
	mode := WorkspaceRW
	if readOnly {
		mode = WorkspaceRO
	}
	return &BindView{
		dest: filepath.Clean(dest),
		view: NewMergedView(source, source, mode, uid, gid),
	}
}

// rel rewrites a path under the bind destination to a path relative to the
// bind source. Paths outside the destination do not route here.
func (b *BindView) rel(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if cleaned == b.dest {
		return ".", nil
	}
	// b.dest+"/" would be "//" for a "/" destination, which no cleaned
	// absolute path starts with; the root bind serves everything.
	if b.dest == "/" {
		return strings.TrimPrefix(cleaned, "/"), nil
	}
	if strings.HasPrefix(cleaned, b.dest+"/") {
		return strings.TrimPrefix(cleaned, b.dest+"/"), nil
	}
	return "", ErrPathOutsideOverlays
}

// rejectSymlinked denies rel paths with any symlinked component under the
// source. session.run writes land live in a rw bind source, so sandbox code
// can plant a link pointing outside the allowed source; mutating through
// it would follow the link out of the bind (#2297 review).
func (b *BindView) rejectSymlinked(rel string) error {
	if rel == "." || rel == "" {
		return nil
	}
	check := b.view.UpperDir
	for _, seg := range strings.Split(filepath.Clean(rel), "/") {
		if seg == "" || seg == "." {
			continue
		}
		check = filepath.Join(check, seg)
		info, err := os.Lstat(check)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink target denied: %s", check)
		}
	}
	return nil
}

func (b *BindView) Stat(path string) (os.FileInfo, error) {
	rel, err := b.rel(path)
	if err != nil {
		return nil, err
	}
	return b.view.Stat(rel)
}

func (b *BindView) ReadFile(path string) ([]byte, error) {
	rel, err := b.rel(path)
	if err != nil {
		return nil, err
	}
	return b.view.ReadFile(rel)
}

func (b *BindView) WriteFile(path string, data []byte, perm os.FileMode) error {
	rel, err := b.rel(path)
	if err != nil {
		return err
	}
	return b.view.WriteFile(rel, data, perm)
}

func (b *BindView) WriteFileReader(path string, r io.Reader, perm os.FileMode) (int64, error) {
	rel, err := b.rel(path)
	if err != nil {
		return 0, err
	}
	return b.view.WriteFileReader(rel, r, perm)
}

func (b *BindView) Remove(path string) error {
	rel, err := b.rel(path)
	if err != nil {
		return err
	}
	if err := b.rejectSymlinked(rel); err != nil {
		return err
	}
	return b.view.Remove(rel)
}

func (b *BindView) RemoveAll(path string) error {
	rel, err := b.rel(path)
	if err != nil {
		return err
	}
	if err := b.rejectSymlinked(rel); err != nil {
		return err
	}
	return b.view.RemoveAll(rel)
}

func (b *BindView) MkdirAll(path string, perm os.FileMode) error {
	rel, err := b.rel(path)
	if err != nil {
		return err
	}
	if err := b.rejectSymlinked(rel); err != nil {
		return err
	}
	return b.view.MkdirAll(rel, perm)
}

func (b *BindView) Rename(oldPath, newPath string) error {
	relOld, err := b.rel(oldPath)
	if err != nil {
		return err
	}
	relNew, err := b.rel(newPath)
	if err != nil {
		return err
	}
	if err := b.rejectSymlinked(relOld); err != nil {
		return err
	}
	if err := b.rejectSymlinked(relNew); err != nil {
		return err
	}
	return b.view.Rename(relOld, relNew)
}

func (b *BindView) Chmod(path string, mode os.FileMode) error {
	rel, err := b.rel(path)
	if err != nil {
		return err
	}
	if err := b.rejectSymlinked(rel); err != nil {
		return err
	}
	return b.view.Chmod(rel, mode)
}

func (b *BindView) ReadDir(path string) ([]os.DirEntry, error) {
	rel, err := b.rel(path)
	if err != nil {
		return nil, err
	}
	return b.view.ReadDir(rel)
}

func (b *BindView) Open(path string) (*os.File, error) {
	rel, err := b.rel(path)
	if err != nil {
		return nil, err
	}
	return b.view.Open(rel)
}

func (b *BindView) Search(root, pattern string) ([]string, error) {
	rel, err := b.rel(root)
	if err != nil {
		return nil, err
	}
	hits, err := b.view.Search(rel, pattern)
	if err != nil {
		return nil, err
	}
	// Hits come back relative to the bind source; re-prefix with the
	// destination so callers routing them back (files search re-stats
	// each hit) reach this bind instead of the workspace view.
	for i, hit := range hits {
		hits[i] = filepath.Join(b.dest, hit)
	}
	return hits, nil
}

func (b *BindView) ReplaceContent(path, old, newStr string) error {
	rel, err := b.rel(path)
	if err != nil {
		return err
	}
	return b.view.ReplaceContent(rel, old, newStr)
}
