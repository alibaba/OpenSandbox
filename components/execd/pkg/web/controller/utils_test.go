// Copyright 2025 Alibaba Group Holding Ltd.
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

package controller

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

func TestDeleteFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "sample.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := DeleteFile(file); err != nil {
		t.Fatalf("DeleteFile returned error: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got err=%v", err)
	}

	// removing a non-existent file should be a no-op
	if err := DeleteFile(file); err != nil {
		t.Fatalf("expected no error deleting missing file, got %v", err)
	}
}

func TestRenameFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dst := filepath.Join(tmp, "nested", "renamed.txt")
	if err := RenameFile(model.RenameFileItem{Src: src, Dest: dst}); err != nil {
		t.Fatalf("RenameFile returned error: %v", err)
	}

	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("expected destination file, got %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected source removed, got err=%v", err)
	}

	// destination exists -> expect error
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}
	if err := RenameFile(model.RenameFileItem{Src: src, Dest: dst}); err == nil {
		t.Fatalf("expected error when destination already exists")
	}
}

func TestSearchFileMetadata(t *testing.T) {
	metadata := map[string]model.FileMetadata{
		"/tmp/a/notes.txt": {Path: "/tmp/a/notes.txt"},
		"/tmp/b/readme.md": {Path: "/tmp/b/readme.md"},
	}

	path, info, ok := SearchFileMetadata(metadata, "/any/notes.txt")
	if !ok {
		t.Fatalf("expected metadata entry")
	}
	if path != "/tmp/a/notes.txt" || info.Path != "/tmp/a/notes.txt" {
		t.Fatalf("unexpected match path=%s info=%v", path, info)
	}

	if _, _, ok := SearchFileMetadata(metadata, "/foo/unknown.txt"); ok {
		t.Fatalf("expected no match")
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		size      int64
		want      []httpRange
		expectErr bool
	}{
		{
			name:   "start-end",
			header: "bytes=0-9",
			size:   20,
			want:   []httpRange{{start: 0, length: 10}},
		},
		{
			name:   "suffix",
			header: "bytes=-5",
			size:   10,
			want:   []httpRange{{start: 5, length: 5}},
		},
		{
			name:      "invalid",
			header:    "bytes=foo",
			size:      10,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRange(tt.header, tt.size)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}
