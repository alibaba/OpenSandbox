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

package runtime

import (
	"reflect"
	"testing"
)

func TestExecuteCodeRequest_SetDefaultHooks(t *testing.T) {
	customResult := func(map[string]any, int) {}

	req := &ExecuteCodeRequest{
		Hooks: ExecuteResultHook{
			OnExecuteResult: customResult,
		},
	}

	req.SetDefaultHooks()

	if req.Hooks.OnExecuteStdout == nil || req.Hooks.OnExecuteStderr == nil || req.Hooks.OnExecuteError == nil {
		t.Fatalf("expected default hooks to be populated")
	}
	if req.Hooks.OnExecuteResult == nil {
		t.Fatalf("expected OnExecuteResult to remain set")
	}
	if reflect.ValueOf(req.Hooks.OnExecuteResult).Pointer() != reflect.ValueOf(customResult).Pointer() {
		t.Fatalf("default hooks should not override existing ones")
	}
}
