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

package proxy

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"knative.dev/pkg/logging"
)

func Test_WebSocketProxy(t *testing.T) {
	// Create mock provider
	provider := &mockProvider{
		endpoints: map[string]string{
			"test-sandbox": "127.0.0.1",
		},
	}

	ctx := context.Background()
	Logger = logging.FromContext(ctx)
	proxy := NewProxy(ctx, provider)

	http.Handle("/ws", proxy)
	proxyPort, err := findAvailablePort()
	proxyURL := "ws://127.0.0.1:" + strconv.Itoa(proxyPort)
	assert.Nil(t, err)

	go func() {
		assert.NoError(t, http.ListenAndServe(":"+strconv.Itoa(proxyPort), nil))
	}()

	time.Sleep(2 * time.Second)

	backendPort, err := findAvailablePort()
	assert.Nil(t, err)

	// backend echo server
	go func() {
		mux2 := http.NewServeMux()
		mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Don't upgrade if original host header isn't preserved
			assert.True(t, strings.HasPrefix(r.Host, "127.0.0.1"))

			conn, err := defaultUpgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println(err)
				return
			}

			messageType, p, err := conn.ReadMessage()
			if err != nil {
				return
			}

			if err = conn.WriteMessage(messageType, p); err != nil {
				return
			}
		})

		err := http.ListenAndServe(":"+strconv.Itoa(backendPort), mux2)
		if err != nil {
			t.Error("ListenAndServe: ", err)
			return
		}
	}()

	time.Sleep(time.Millisecond * 100)

	// frontend server, dial now our proxy, which will reverse proxy our
	// message to the backend websocket server.
	h := http.Header{}
	h.Set(SandboxIngress, "test-sandbox-"+strconv.Itoa(backendPort))
	conn, _, err := websocket.DefaultDialer.Dial(proxyURL+"/ws", h)
	if err != nil {
		t.Fatal(err)
	}

	// write a message and send it to the backend server
	msg := "hello kite"
	err = conn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		t.Error(err)
	}

	messageType, p, err := conn.ReadMessage()
	if err != nil {
		t.Error(err)
	}

	if messageType != websocket.TextMessage {
		t.Error("incoming message type is not Text")
	}

	if msg != string(p) {
		t.Errorf("expecting: %s, got: %s", msg, string(p))
	}
}
