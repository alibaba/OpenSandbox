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

package task_executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Reset_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reset" {
			t.Errorf("expected path /reset, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		resp := ResetResponse{
			Status:  ResetStatusSuccess,
			Message: "Reset completed",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Reset(context.Background(), &ResetRequest{})
	if err != nil {
		t.Errorf("Reset() error = %v", err)
		return
	}
	if resp.Status != ResetStatusSuccess {
		t.Errorf("Reset() status = %v, want %v", resp.Status, ResetStatusSuccess)
	}
}

func TestClient_Reset_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.Reset(context.Background(), &ResetRequest{})
	if err == nil {
		t.Error("Reset() should return error on server error")
	}
}

func TestClient_Reset_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClientWithTimeout(server.URL, 10*time.Millisecond)
	_, err := client.Reset(context.Background(), &ResetRequest{})
	if err == nil {
		t.Error("Reset() should return error on timeout")
	}
}

func TestClient_Reset_NilClient(t *testing.T) {
	var client *Client
	_, err := client.Reset(context.Background(), &ResetRequest{})
	if err == nil {
		t.Error("Reset() should return error for nil client")
	}
}
