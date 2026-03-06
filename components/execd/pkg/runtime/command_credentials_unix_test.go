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

//go:build !windows
// +build !windows

package runtime

import (
	"os"
	"os/exec"
	"testing"
)

func TestApplyCommandSysProcAttr_UsesCurrentIdentityFallback(t *testing.T) {
	cmd := exec.Command("bash", "-lc", "true")
	gid := uint32(2002)

	err := applyCommandSysProcAttr(cmd, &ExecuteCodeRequest{Gid: &gid})
	if err != nil {
		t.Fatalf("applyCommandSysProcAttr returned error: %v", err)
	}
	if cmd.SysProcAttr == nil {
		t.Fatalf("expected SysProcAttr to be populated")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Fatalf("expected Setpgid to be true")
	}
	if cmd.SysProcAttr.Credential == nil {
		t.Fatalf("expected Credential to be set when gid is provided")
	}
	if cmd.SysProcAttr.Credential.Uid != uint32(os.Getuid()) {
		t.Fatalf("expected uid fallback to current process uid")
	}
	if cmd.SysProcAttr.Credential.Gid != gid {
		t.Fatalf("expected gid %d, got %d", gid, cmd.SysProcAttr.Credential.Gid)
	}
}
