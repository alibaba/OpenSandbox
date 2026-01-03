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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/opensandbox/router/pkg/flag"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/logging"
)

func Test_HTTPProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(realBackendHTTPHandler))
	defer server.Close()
	serverPort := server.URL[len("http://127.0.0.1:"):]

	pod1 := MakePod().
		Name("sandbox-1").
		Namespace(flag.Namespace).
		IP("127.0.0.1").
		Phase(corev1.PodRunning).
		Label(flag.IngressLabelKey, strings.ReplaceAll(uuid.New().String(), "-", "")).
		Obj()

	clientset := fake.NewSimpleClientset(pod1)

	ctx := context.WithValue(context.Background(), kubeclient.Key{}, clientset)
	Logger = logging.FromContext(ctx)
	proxy := NewProxy(ctx)

	http.Handle("/", proxy)
	port, err := findAvailablePort()
	assert.Nil(t, err)

	go func() {
		assert.NoError(t, http.ListenAndServe(":"+strconv.Itoa(port), nil))
	}()

	time.Sleep(2 * time.Second)

	// no header
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%v/hello", port), nil)
	assert.Nil(t, err)
	response, err := http.DefaultClient.Do(request)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotAcceptable, response.StatusCode)
	bytes, _ := io.ReadAll(response.Body)
	t.Log(string(bytes))

	// no pod backend
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%v/hello", port), nil)
	request.Header.Set(SandboxIngress, fmt.Sprintf("%s-%v", uuid.New().String(), port))
	response, err = http.DefaultClient.Do(request)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
	bytes, _ = io.ReadAll(response.Body)
	t.Log(string(bytes))

	// has two pod
	session := strings.ReplaceAll(uuid.New().String(), "-", "")
	_, err = clientset.CoreV1().Pods(flag.Namespace).Create(ctx, MakePod().
		Name("sandbox-2").
		Namespace(flag.Namespace).
		IP("127.0.0.1").
		Phase(corev1.PodRunning).
		Label(flag.IngressLabelKey, session).
		Obj(), metav1.CreateOptions{})
	assert.Nil(t, err)
	_, err = clientset.CoreV1().Pods(flag.Namespace).Create(ctx, MakePod().
		Name("sandbox-3").
		Namespace(flag.Namespace).
		IP("127.0.0.1").
		Phase(corev1.PodRunning).
		Label(flag.IngressLabelKey, session).
		Obj(), metav1.CreateOptions{})
	assert.Nil(t, err)

	request, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%v/hello", port), nil)
	request.Header.Set(SandboxIngress, fmt.Sprintf("%s-%v", session, port))
	response, err = http.DefaultClient.Do(request)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusConflict, response.StatusCode)
	bytes, _ = io.ReadAll(response.Body)
	t.Log(string(bytes))

	// current pod backend
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%v/hello?a=1&b=2", port), nil)
	assert.Nil(t, err)

	request.Header.Set(SandboxIngress, fmt.Sprintf("%s-%v", pod1.Labels[flag.IngressLabelKey], serverPort))
	response, err = http.DefaultClient.Do(request)
	assert.Nil(t, err)
	if response.StatusCode != http.StatusOK {
		bytes, err := io.ReadAll(response.Body)
		assert.Nil(t, err)
		t.Log(string(bytes))
	}
	assert.Equal(t, http.StatusOK, response.StatusCode)

	// // Compatible Host parsing for reverse proxy mode
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%v/hello?a=1&b=2", port), nil)
	assert.Nil(t, err)

	request.Host = fmt.Sprintf("%s-%v.sandbox.alibaba-inc.com", pod1.Labels[flag.IngressLabelKey], serverPort)
	response, err = http.DefaultClient.Do(request)
	assert.Nil(t, err)
	if response.StatusCode != http.StatusOK {
		bytes, err := io.ReadAll(response.Body)
		assert.Nil(t, err)
		t.Log(string(bytes))
	}
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func realBackendHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path != "/hello" {
		http.Error(w, fmt.Sprintf("path is not /hello, but %s", r.URL.Path), http.StatusBadRequest)
	}
	if r.URL.RawQuery != "a=1&b=2" {
		http.Error(w, fmt.Sprintf("query is not a=1&b=2, but %s", r.URL.RawQuery), http.StatusBadRequest)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("hello world"))
}
