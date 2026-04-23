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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
)

func newTestSnapshotReconciler(objs ...client.Object) *SandboxSnapshotReconciler {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(sandboxv1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sandboxv1alpha1.SandboxSnapshot{}).
		WithObjects(objs...).
		Build()

	return &SandboxSnapshotReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}
}

func TestSandboxSnapshotHandleCommitting_SetsSucceedReadyCondition(t *testing.T) {
	snapshot := &sandboxv1alpha1.SandboxSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Status: sandboxv1alpha1.SandboxSnapshotStatus{
			Phase: sandboxv1alpha1.SandboxSnapshotPhaseCommitting,
			Containers: []sandboxv1alpha1.ContainerSnapshot{
				{ContainerName: "main", ImageURI: "registry/test:tag"},
			},
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot-commit",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	r := newTestSnapshotReconciler(snapshot, job)

	result, err := r.handleCommitting(context.Background(), snapshot)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &sandboxv1alpha1.SandboxSnapshot{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "test-snapshot", Namespace: "default"}, updated))
	assert.Equal(t, sandboxv1alpha1.SandboxSnapshotPhaseSucceed, updated.Status.Phase)

	foundReady := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == sandboxv1alpha1.SandboxSnapshotConditionReady {
			foundReady = true
			assert.Equal(t, sandboxv1alpha1.ConditionTrue, cond.Status)
			assert.Equal(t, "SnapshotReady", cond.Reason)
			assert.NotNil(t, cond.LastTransitionTime)
		}
		if cond.Type == sandboxv1alpha1.SandboxSnapshotConditionFailed {
			assert.NotEqual(t, sandboxv1alpha1.ConditionTrue, cond.Status)
		}
	}
	assert.True(t, foundReady, "Ready condition should be set after successful commit")
}

func TestSandboxSnapshotHandleCommitting_KeepsRetryingWhenJobHasNotTerminallyFailed(t *testing.T) {
	snapshot := &sandboxv1alpha1.SandboxSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Status: sandboxv1alpha1.SandboxSnapshotStatus{
			Phase: sandboxv1alpha1.SandboxSnapshotPhaseCommitting,
			Containers: []sandboxv1alpha1.ContainerSnapshot{
				{ContainerName: "main", ImageURI: "registry/test:tag"},
			},
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot-commit",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Active: 1,
			Failed: 1,
		},
	}

	r := newTestSnapshotReconciler(snapshot, job)

	result, err := r.handleCommitting(context.Background(), snapshot)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)

	updated := &sandboxv1alpha1.SandboxSnapshot{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "test-snapshot", Namespace: "default"}, updated))
	assert.Equal(t, sandboxv1alpha1.SandboxSnapshotPhaseCommitting, updated.Status.Phase)
}

func TestSandboxSnapshotHandlePending_MissingRegistrySetsFailedCondition(t *testing.T) {
	snapshot := &sandboxv1alpha1.SandboxSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Status: sandboxv1alpha1.SandboxSnapshotStatus{
			Phase: sandboxv1alpha1.SandboxSnapshotPhasePending,
		},
	}

	r := newTestSnapshotReconciler(snapshot)

	result, err := r.handlePending(context.Background(), snapshot)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &sandboxv1alpha1.SandboxSnapshot{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "test-snapshot", Namespace: "default"}, updated))
	assert.Equal(t, sandboxv1alpha1.SandboxSnapshotPhaseFailed, updated.Status.Phase)

	foundFailed := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == sandboxv1alpha1.SandboxSnapshotConditionFailed {
			foundFailed = true
			assert.Equal(t, sandboxv1alpha1.ConditionTrue, cond.Status)
			assert.Equal(t, "RegistryNotConfigured", cond.Reason)
			assert.Contains(t, cond.Message, "snapshot-registry")
		}
	}
	assert.True(t, foundFailed, "Failed condition should be set when registry config is missing")
}

func TestBuildCommitJob_SetsBoundedBackoffLimit(t *testing.T) {
	snapshot := &sandboxv1alpha1.SandboxSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Status: sandboxv1alpha1.SandboxSnapshotStatus{
			SourcePodName:  "test-pod",
			SourceNodeName: "node-1",
			Containers: []sandboxv1alpha1.ContainerSnapshot{
				{
					ContainerName: "main",
					ImageURI:      "registry.example.com/test:tag",
				},
			},
		},
	}

	r := newTestSnapshotReconciler(snapshot)
	r.SnapshotPushSecret = "registry-snapshot-push-secret"

	job, err := r.buildCommitJob(snapshot)
	require.NoError(t, err)
	require.NotNil(t, job.Spec.BackoffLimit)
	assert.Equal(t, DefaultCommitJobBackoffLimit, *job.Spec.BackoffLimit)
	assert.Equal(t, fmt.Sprintf("%s-commit", snapshot.Name), job.Name)
}
