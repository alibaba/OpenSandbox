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

package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// buildUnstructuredSandbox creates a minimal unstructured Sandbox object.
func buildUnstructuredSandbox(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": agentSandboxGroup + "/" + agentSandboxVersion,
			"kind":       "Sandbox",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"podTemplate": map[string]any{
					"spec": map[string]any{
						"containers": []any{},
					},
					"metadata": map[string]any{},
				},
			},
		},
	}
}

func TestAgentSandboxProvider_Start_Success(t *testing.T) {
	namespace := "test-ns"

	obj := buildUnstructuredSandbox("demo", namespace)
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    agentSandboxGroup,
		Version:  agentSandboxVersion,
		Resource: agentSandboxResource,
	}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			gvr: "SandboxList",
		},
		obj,
	)

	provider := newAgentSandboxProviderWithClient(fakeDyn, namespace, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := provider.Start(ctx)
	assert.NoError(t, err, "Start should succeed with fake dynamic informer")

	// Manually seed store (fake dynamic client doesn't backfill informer cache automatically)
	err = provider.informer.GetStore().Add(obj)
	assert.NoError(t, err)

	key := obj.GetNamespace() + "/" + obj.GetName()
	_, exists, _ := provider.informer.GetStore().GetByKey(key)
	assert.True(t, exists, "informer cache should accept added object after start")
}

func TestAgentSandboxProvider_Start_ContextCancelled(t *testing.T) {
	namespace := "test-ns"

	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    agentSandboxGroup,
		Version:  agentSandboxVersion,
		Resource: agentSandboxResource,
	}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			gvr: "SandboxList",
		},
	)

	provider := newAgentSandboxProviderWithClient(fakeDyn, namespace, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before start

	err := provider.Start(ctx)
	assert.Error(t, err, "Start should fail when context already cancelled")
}

func TestAgentSandboxProvider_GetEndpoint_ServiceFQDN(t *testing.T) {
	namespace := "test-ns"
	obj := buildUnstructuredSandbox("demo", namespace)
	obj.Object["status"] = map[string]any{
		"serviceFQDN": "sandbox.demo.svc.cluster.local",
		"conditions": []any{
			map[string]any{
				"type":   "Ready",
				"status": "True",
			},
		},
	}

	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    agentSandboxGroup,
		Version:  agentSandboxVersion,
		Resource: agentSandboxResource,
	}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			gvr: "SandboxList",
		},
	)

	provider := newAgentSandboxProviderWithClient(fakeDyn, namespace, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := provider.Start(ctx)
	assert.NoError(t, err)

	// Seed store
	err = provider.informer.GetStore().Add(obj)
	assert.NoError(t, err)

	endpoint, err := provider.GetEndpoint("demo")
	assert.NoError(t, err)
	assert.Equal(t, "sandbox.demo.svc.cluster.local", endpoint)
}

func TestAgentSandboxProvider_GetEndpoint_NotFound(t *testing.T) {
	namespace := "test-ns"

	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    agentSandboxGroup,
		Version:  agentSandboxVersion,
		Resource: agentSandboxResource,
	}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			gvr: "SandboxList",
		},
	)

	provider := newAgentSandboxProviderWithClient(fakeDyn, namespace, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := provider.Start(ctx)
	assert.NoError(t, err)

	_, err = provider.GetEndpoint("missing")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSandboxNotFound))
}

func TestAgentSandboxProvider_GetEndpoint_NoServiceFQDN(t *testing.T) {
	namespace := "test-ns"
	obj := buildUnstructuredSandbox("demo", namespace)
	obj.Object["status"] = map[string]any{}

	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    agentSandboxGroup,
		Version:  agentSandboxVersion,
		Resource: agentSandboxResource,
	}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			gvr: "SandboxList",
		},
	)

	provider := newAgentSandboxProviderWithClient(fakeDyn, namespace, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := provider.Start(ctx)
	assert.NoError(t, err)

	// Seed store
	err = provider.informer.GetStore().Add(obj)
	assert.NoError(t, err)

	_, err = provider.GetEndpoint("demo")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSandboxNotReady))
}

func TestAgentSandboxProvider_GetEndpoint_NotReadyCondition(t *testing.T) {
	namespace := "test-ns"
	obj := buildUnstructuredSandbox("demo", namespace)
	obj.Object["status"] = map[string]any{
		"serviceFQDN": "sandbox.demo.svc.cluster.local",
		"conditions": []any{
			map[string]any{
				"type":    "Ready",
				"status":  "False",
				"reason":  "DependenciesNotReady",
				"message": "Pod not ready",
			},
		},
	}

	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    agentSandboxGroup,
		Version:  agentSandboxVersion,
		Resource: agentSandboxResource,
	}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			gvr: "SandboxList",
		},
	)

	provider := newAgentSandboxProviderWithClient(fakeDyn, namespace, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := provider.Start(ctx)
	assert.NoError(t, err)

	// Seed store
	err = provider.informer.GetStore().Add(obj)
	assert.NoError(t, err)

	_, err = provider.GetEndpoint("demo")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSandboxNotReady))
}
