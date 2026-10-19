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

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/opensandbox/execd/pkg/isolation"
)

// Review-fix regression tests for #2297: symlink-parent escapes in rw binds,
// search-hit re-prefixing, bwrap mount shadowing, and root bind destinations.

func newBindSession(t *testing.T, overlays []IsolatedOverlayOptions, binds []isolation.BindMount) (*isolatedSession, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "ws"), 0o755); err != nil {
		t.Fatal(err)
	}
	if overlays == nil {
		overlays = []IsolatedOverlayOptions{{Path: filepath.Join(base, "ws"), Mode: "rw"}}
	}
	s := newIsolatedSession("bind-review", &IsolatedSessionOptions{
		Overlays: overlays,
		Binds:    binds,
	}, newStubIsolator(), nil)
	return s, base
}

// A symlink created by the sandbox inside a rw bind must not let files-API
// mutations escape the bind source through a symlinked parent.
func TestNewMergedView_BindRejectsSymlinkedParents(t *testing.T) {
	s, base := newBindSession(t, nil, nil)
	bindSource := filepath.Join(base, "esc-src")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{bindSource, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "victim.txt"), []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The sandbox (session.run writes land live in the bind source) plants
	// a symlink pointing outside the allowed source.
	if err := os.Symlink(outside, filepath.Join(bindSource, "link")); err != nil {
		t.Fatal(err)
	}
	s.opts.Binds = []isolation.BindMount{{Source: bindSource, Dest: "/data", ReadOnly: false}}

	view := newMergedView(s)

	if err := view.Remove("/data/link/victim.txt"); err == nil {
		t.Fatal("remove through symlinked parent accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "victim.txt")); err != nil {
		t.Fatalf("victim deleted through symlink escape: %v", err)
	}
	if err := view.RemoveAll("/data/link"); err == nil {
		t.Fatal("removeall through symlinked parent accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "victim.txt")); err != nil {
		t.Fatalf("victim deleted through symlink escape (removeall): %v", err)
	}
	if err := view.Chmod("/data/link/victim.txt", 0o777); err == nil {
		t.Fatal("chmod through symlinked parent accepted")
	}
	if err := view.MkdirAll("/data/link/newdir", 0o755); err == nil {
		t.Fatal("mkdirall through symlinked parent accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "newdir")); !os.IsNotExist(err) {
		t.Fatal("directory created through symlink escape")
	}
	if err := view.Rename("/data/link/victim.txt", "/data/renamed.txt"); err == nil {
		t.Fatal("rename with symlinked source parent accepted")
	}
	if err := view.Rename("/data/stays.txt", "/data/link/moved.txt"); err == nil {
		t.Fatal("rename with symlinked destination parent accepted")
	}
}

// Search hits rooted at a bind must come back dest-prefixed so they route
// through the router to the bind, not to the workspace.
func TestNewMergedView_BindSearchPrefixesDest(t *testing.T) {
	s, base := newBindSession(t, nil, nil)
	bindSource := filepath.Join(base, "search-src")
	if err := os.MkdirAll(bindSource, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bindSource, "hit.txt"), []byte("h"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "ws", "decoy.txt"), []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.opts.Binds = []isolation.BindMount{{Source: bindSource, Dest: "/data", ReadOnly: false}}

	view := newMergedView(s)

	hits, err := view.Search("/data", "*.txt")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %v, want exactly the bind hit", hits)
	}
	if hits[0] != "/data/hit.txt" {
		t.Fatalf("hit = %q, want /data/hit.txt (dest-prefixed)", hits[0])
	}
	// The returned path must route back to the bind, not the workspace.
	if _, err := view.Stat(hits[0]); err != nil {
		t.Fatalf("hit %q does not route: %v", hits[0], err)
	}
}

// Mirror bwrap shadowing: mounts apply in argv order (overlays shallow→deep,
// then binds in option order) and a later mount shadows earlier ones in its
// subtree — not the longest prefix.
func TestNewMergedView_BindShadowingMirrorsBwrap(t *testing.T) {
	s, base := newBindSession(t, nil, nil)
	srcA := filepath.Join(base, "a-src") // earlier, deeper dest
	srcB := filepath.Join(base, "b-src") // later, shallower dest: shadows A
	for _, dir := range []string{srcA, srcB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.opts.Binds = []isolation.BindMount{
		{Source: srcA, Dest: "/data/sub", ReadOnly: false},
		{Source: srcB, Dest: "/data", ReadOnly: false},
	}

	view := newMergedView(s)

	// The later /data bind shadows the earlier /data/sub bind: run would
	// serve srcB for /data/sub/x, so the files API must too.
	if err := view.WriteFile("/data/sub/x.txt", []byte("b"), 0o600); err != nil {
		t.Fatalf("write under shadowed subtree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcB, "sub", "x.txt")); err != nil {
		t.Fatalf("write did not land in the shadowing (later) bind source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcA, "x.txt")); !os.IsNotExist(err) {
		t.Fatal("write leaked into the shadowed (earlier) bind source")
	}

	// A bind also shadows an overlay mounted at the same or ancestor path
	// (binds are argv-section 6, overlays section 4).
	ws := filepath.Join(base, "ws")
	if err := os.WriteFile(filepath.Join(ws, "overlay-file.txt"), []byte("o"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, base2 := newBindSession(t, []IsolatedOverlayOptions{{Path: ws, Mode: "rw"}}, nil)
	overlayBind := filepath.Join(base2, "ov-src")
	if err := os.MkdirAll(overlayBind, 0o755); err != nil {
		t.Fatal(err)
	}
	s2.opts.Binds = []isolation.BindMount{{Source: overlayBind, Dest: ws, ReadOnly: false}}
	view2 := newMergedView(s2)

	if err := view2.WriteFile(filepath.Join(ws, "bind-wins.txt"), []byte("b"), 0o600); err != nil {
		t.Fatalf("write under bind-over-overlay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(overlayBind, "bind-wins.txt")); err != nil {
		t.Fatalf("write did not land in the bind source: %v", err)
	}
}

// A root bind destination is accepted by validation and mounted by bwrap;
// the files API must serve it (it shadows every earlier caller mount).
func TestNewMergedView_RootBindServes(t *testing.T) {
	s, base := newBindSession(t, nil, nil)
	rootSrc := filepath.Join(base, "root-src")
	if err := os.MkdirAll(rootSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	s.opts.Binds = []isolation.BindMount{{Source: rootSrc, Dest: "/", ReadOnly: false}}

	view := newMergedView(s)

	if err := view.WriteFile("/anywhere/x.txt", []byte("r"), 0o600); err != nil {
		t.Fatalf("write under root bind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootSrc, "anywhere", "x.txt")); err != nil {
		t.Fatalf("write did not land in the root bind source: %v", err)
	}
}
