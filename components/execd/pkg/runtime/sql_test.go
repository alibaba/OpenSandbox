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
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
)

func TestExecuteSelectSQLQuery_Success(t *testing.T) {
	driver := &stubDriver{
		columns: []string{"id", "name"},
		rows: [][]driver.Value{
			{int64(1), "alice"},
			{int64(2), "bob"},
		},
	}
	db := newStubDB(t, driver)

	c := NewController("", "")
	c.db = db

	var (
		gotResult map[string]any
		gotError  *execute.ErrorOutput
		completed bool
	)

	req := &ExecuteCodeRequest{
		Code: "SELECT * FROM users",
		Hooks: ExecuteResultHook{
			OnExecuteResult: func(result map[string]any, _ int) {
				gotResult = result
			},
			OnExecuteError: func(err *execute.ErrorOutput) {
				gotError = err
			},
			OnExecuteComplete: func(time.Duration) {
				completed = true
			},
		},
	}

	if err := c.executeSelectSQLQuery(context.Background(), req); err != nil {
		t.Fatalf("executeSelectSQLQuery returned error: %v", err)
	}

	if gotError != nil {
		t.Fatalf("unexpected error hook: %+v", gotError)
	}
	if !completed {
		t.Fatalf("expected completion hook to be triggered")
	}

	raw, ok := gotResult["text/plain"]
	if !ok {
		t.Fatalf("expected text/plain payload")
	}
	var qr QueryResult
	if err := json.Unmarshal([]byte(raw.(string)), &qr); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(qr.Columns) != 2 || qr.Columns[0] != "id" || qr.Columns[1] != "name" {
		t.Fatalf("unexpected columns: %#v", qr.Columns)
	}
	if len(qr.Rows) != 2 || qr.Rows[0][0] != "1" || qr.Rows[1][1] != "bob" {
		t.Fatalf("unexpected rows: %#v", qr.Rows)
	}
}

func TestExecuteUpdateSQLQuery_Success(t *testing.T) {
	driver := &stubDriver{
		execRowsAffected: 3,
	}
	db := newStubDB(t, driver)

	c := NewController("", "")
	c.db = db

	var (
		gotResult map[string]any
		gotError  *execute.ErrorOutput
		completed bool
	)

	req := &ExecuteCodeRequest{
		Code: "UPDATE users SET name='alice' WHERE id=1",
		Hooks: ExecuteResultHook{
			OnExecuteResult: func(result map[string]any, _ int) {
				gotResult = result
			},
			OnExecuteError: func(err *execute.ErrorOutput) {
				gotError = err
			},
			OnExecuteComplete: func(time.Duration) {
				completed = true
			},
		},
	}

	if err := c.executeUpdateSQLQuery(context.Background(), req); err != nil {
		t.Fatalf("executeUpdateSQLQuery returned error: %v", err)
	}

	if gotError != nil {
		t.Fatalf("unexpected error hook: %+v", gotError)
	}
	if !completed {
		t.Fatalf("expected completion hook to be triggered")
	}

	raw, ok := gotResult["text/plain"]
	if !ok {
		t.Fatalf("expected text/plain payload")
	}
	var qr QueryResult
	if err := json.Unmarshal([]byte(raw.(string)), &qr); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(qr.Columns) != 1 || qr.Columns[0] != "affected_rows" {
		t.Fatalf("unexpected columns: %#v", qr.Columns)
	}
	if len(qr.Rows) != 1 || len(qr.Rows[0]) != 1 || qr.Rows[0][0] != float64(3) {
		t.Fatalf("unexpected affected rows: %#v", qr.Rows)
	}
}
