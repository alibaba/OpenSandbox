package proxy

import (
	"context"
	"github.com/alibaba/opensandbox/router/pkg/flag"
	"log"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	logging2 "knative.dev/pkg/logging"
)

func Test_WebSocketProxy(t *testing.T) {
	pod1 := MakePod().
		Name("sandbox-x").
		Namespace(flag.Namespace).
		IP("127.0.0.1").
		Phase(corev1.PodPending).
		Label(flag.IngressLabelKey, strings.ReplaceAll(uuid.New().String(), "-", "")).
		Obj()

	clientset := fake.NewSimpleClientset(pod1)

	ctx := context.WithValue(context.Background(), kubeclient.Key{}, clientset)
	Logger = logging2.FromContext(ctx)
	proxy := NewProxy(ctx)

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
	h.Set(SandboxIngress, pod1.Labels[flag.IngressLabelKey]+"-"+strconv.Itoa(backendPort))
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
