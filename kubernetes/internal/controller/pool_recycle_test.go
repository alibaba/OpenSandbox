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

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
)

func TestInjectTaskExecutor_BasicInjection(t *testing.T) {
	pool := &sandboxv1alpha1.Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: sandboxv1alpha1.PoolSpec{
			PodRecyclePolicy: sandboxv1alpha1.PodRecyclePolicyReuse,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Image: "test-image"},
			},
		},
	}

	r := &PoolReconciler{
		TaskExecutorImage:     "task-executor:latest",
		TaskExecutorResources: "200m,128Mi",
	}

	r.injectTaskExecutor(pod, pool)

	// Verify ShareProcessNamespace is true
	if pod.Spec.ShareProcessNamespace == nil || !*pod.Spec.ShareProcessNamespace {
		t.Error("ShareProcessNamespace should be true")
	}

	// Verify task-executor container is injected
	found := false
	for _, c := range pod.Spec.Containers {
		if c.Name == "task-executor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("task-executor container should be injected")
	}
}

func TestInjectTaskExecutor_RestartPolicyChange(t *testing.T) {
	pool := &sandboxv1alpha1.Pool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: sandboxv1alpha1.PoolSpec{
			PodRecyclePolicy: sandboxv1alpha1.PodRecyclePolicyReuse,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{Name: "main", Image: "test-image"},
			},
		},
	}

	r := &PoolReconciler{
		TaskExecutorImage:     "task-executor:latest",
		TaskExecutorResources: "200m,128Mi",
	}

	r.injectTaskExecutor(pod, pool)

	// Verify RestartPolicy changed to Always
	if pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Errorf("RestartPolicy should be Always, got %s", pod.Spec.RestartPolicy)
	}
}
