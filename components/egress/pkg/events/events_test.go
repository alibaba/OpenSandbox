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

package events

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type captureSubscriber struct {
	recv chan BlockedEvent
}

func (c *captureSubscriber) HandleBlocked(_ context.Context, ev BlockedEvent) {
	c.recv <- ev
}

type blockingSubscriber struct {
	block chan struct{}
}

func (b *blockingSubscriber) HandleBlocked(_ context.Context, ev BlockedEvent) {
	// Block until the channel is closed to simulate a slow consumer and trigger backpressure.
	<-b.block
	_ = ev
}

func TestBroadcasterFanout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := NewBroadcaster(ctx, BroadcasterConfig{QueueSize: 2})

	sub1 := &captureSubscriber{recv: make(chan BlockedEvent, 1)}
	sub2 := &captureSubscriber{recv: make(chan BlockedEvent, 1)}
	b.AddSubscriber(sub1)
	b.AddSubscriber(sub2)

	ev := BlockedEvent{Hostname: "example.com.", Timestamp: time.Now()}
	b.Publish(ev)

	select {
	case got := <-sub1.recv:
		if got.Hostname != ev.Hostname {
			t.Fatalf("sub1 expected hostname %s, got %s", ev.Hostname, got.Hostname)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub1 did not receive event")
	}

	select {
	case got := <-sub2.recv:
		if got.Hostname != ev.Hostname {
			t.Fatalf("sub2 expected hostname %s, got %s", ev.Hostname, got.Hostname)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub2 did not receive event")
	}

	b.Close()
}

func TestBroadcasterDropsWhenSubscriberBackedUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Small queue; blocking subscriber will hold the first event.
	b := NewBroadcaster(ctx, BroadcasterConfig{QueueSize: 1})
	block := make(chan struct{})
	sub := &blockingSubscriber{block: block}
	b.AddSubscriber(sub)

	ev1 := BlockedEvent{Hostname: "first.example", Timestamp: time.Now()}
	ev2 := BlockedEvent{Hostname: "second.example", Timestamp: time.Now()}

	b.Publish(ev1)
	// This publish should drop because subscriber is blocked and queue size is 1.
	b.Publish(ev2)

	// Allow subscriber to drain and exit.
	close(block)

	b.Close()
}

func TestWebhookSubscriberSendsPayload(t *testing.T) {
	var (
		gotMethod  string
		gotPayload webhookPayload
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sub := NewWebhookSubscriber(server.URL)
	if sub == nil {
		t.Fatal("webhook subscriber should not be nil")
	}

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ev := BlockedEvent{Hostname: "Example.com.", Timestamp: ts}
	sub.HandleBlocked(context.Background(), ev)

	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPayload.Hostname != ev.Hostname {
		t.Fatalf("expected hostname %s, got %s", ev.Hostname, gotPayload.Hostname)
	}
	if gotPayload.Source != webhookSource {
		t.Fatalf("expected source %s, got %s", webhookSource, gotPayload.Source)
	}
	if gotPayload.Timestamp == "" {
		t.Fatalf("expected timestamp to be set")
	}
}
