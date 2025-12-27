package proxy

import (
	"context"
	"github.com/alibaba/opensandbox/router/pkg/flag"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func Test_WatchPods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(realBackendHTTPHandler))
	defer server.Close()

	multiSession := uuid.New().String()
	pod1 := MakePod().
		Name("session-pod-1").
		Namespace(flag.Namespace).
		IP("127.0.0.1").
		Phase(corev1.PodRunning).
		Label(flag.IngressLabelKey, multiSession).
		Obj()
	pod2 := MakePod().
		Name("session-pod-2").
		Namespace(flag.Namespace).
		IP(""). // pod ip is empty
		Phase(corev1.PodPending).
		Label(flag.IngressLabelKey, uuid.New().String()).
		Obj()
	pod3 := MakePod().
		Name("other-pods").
		Namespace(flag.Namespace).
		Phase(corev1.PodRunning).
		Obj()
	pod4 := MakePod().
		Name("session-pod-4").
		Namespace(flag.Namespace).
		IP("127.0.0.1"). // pod ip is empty
		Phase(corev1.PodFailed).
		Label(flag.IngressLabelKey, multiSession).
		Obj()

	clientset := fake.NewSimpleClientset(pod1, pod2, pod3, pod4)

	ctx := context.WithValue(context.Background(), kubeclient.Key{}, clientset)
	proxy := NewProxy(ctx)

	time.Sleep(100 * time.Millisecond)
	_, err := proxy.lister.Pods(flag.Namespace).Get(pod1.Name)
	assert.Nil(t, err)
	_, err = proxy.lister.Pods(flag.Namespace).Get(pod2.Name)
	assert.Nil(t, err)
	err = clientset.CoreV1().Pods(flag.Namespace).Delete(ctx, pod2.Name, metav1.DeleteOptions{})
	assert.Nil(t, err)

	time.Sleep(100 * time.Millisecond)
	_, err = proxy.lister.Pods(flag.IngressLabelKey).Get(pod2.Name)
	assert.True(t, errors.IsNotFound(err))
	err = clientset.CoreV1().Pods(flag.Namespace).Delete(ctx, pod1.Name, metav1.DeleteOptions{})
	assert.Nil(t, err)

	time.Sleep(100 * time.Millisecond)
	_, err = proxy.lister.Pods(flag.Namespace).Get(pod1.Name)
	assert.True(t, errors.IsNotFound(err))
}

func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	return port, nil
}
