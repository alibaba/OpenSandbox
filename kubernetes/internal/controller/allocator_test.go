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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAllocatorSchedule(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockAllocationStore(ctrl)
	syncer := NewMockAllocationSyncer(ctrl)
	allocator := &defaultAllocator{
		store:  store,
		syncer: syncer,
	}
	replica1 := int32(1)
	replica2 := int32(2)
	type TestCase struct {
		name         string
		spec         *AllocSpec
		poolAlloc    *PoolAllocation
		sandboxAlloc *SandboxAllocation
		release      *AllocationRelease
		wantStatus   *AllocStatus
	}
	cases := []TestCase{
		{
			name: "normal",
			spec: &AllocSpec{
				Pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod2",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
				Pool: &sandboxv1alpha1.Pool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool1",
					},
				},
				Sandboxes: []*sandboxv1alpha1.BatchSandbox{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "sbx1",
						},
						Spec: sandboxv1alpha1.BatchSandboxSpec{
							PoolRef:  "pool1",
							Replicas: &replica1,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "sbx2",
						},
						Spec: sandboxv1alpha1.BatchSandboxSpec{
							PoolRef:  "pool1",
							Replicas: &replica1,
						},
					},
				},
			},
			poolAlloc: &PoolAllocation{
				PodAllocation: map[string]string{},
			},
			sandboxAlloc: &SandboxAllocation{
				Pods: []string{},
			},
			release: &AllocationRelease{
				Pods: []string{},
			},
			wantStatus: &AllocStatus{
				PodAllocation: map[string]string{
					"pod1": "sbx1",
					"pod2": "sbx2",
				},
				PodSupplement: 0,
			},
		},
		{
			name: "pod not running",
			spec: &AllocSpec{
				Pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod2",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodPending,
						},
					},
				},
				Pool: &sandboxv1alpha1.Pool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool1",
					},
				},
				Sandboxes: []*sandboxv1alpha1.BatchSandbox{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "sbx1",
						},
						Spec: sandboxv1alpha1.BatchSandboxSpec{
							PoolRef:  "pool1",
							Replicas: &replica1,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "sbx2",
						},
						Spec: sandboxv1alpha1.BatchSandboxSpec{
							PoolRef:  "pool1",
							Replicas: &replica1,
						},
					},
				},
			},
			poolAlloc: &PoolAllocation{
				PodAllocation: map[string]string{},
			},
			sandboxAlloc: &SandboxAllocation{
				Pods: []string{},
			},
			release: &AllocationRelease{
				Pods: []string{},
			},
			wantStatus: &AllocStatus{
				PodAllocation: map[string]string{
					"pod1": "sbx1",
				},
				PodSupplement: 1,
			},
		},
		{
			name: "already partial allocated",
			spec: &AllocSpec{
				Pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod2",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod3",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
				Pool: &sandboxv1alpha1.Pool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool1",
					},
				},
				Sandboxes: []*sandboxv1alpha1.BatchSandbox{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "sbx1",
						},
						Spec: sandboxv1alpha1.BatchSandboxSpec{
							PoolRef:  "pool1",
							Replicas: &replica2,
						},
					},
				},
			},
			poolAlloc: &PoolAllocation{
				PodAllocation: map[string]string{
					"pod1": "sbx1",
				},
			},
			sandboxAlloc: &SandboxAllocation{
				Pods: []string{
					"pod1",
				},
			},
			release: &AllocationRelease{
				Pods: []string{},
			},
			wantStatus: &AllocStatus{
				PodAllocation: map[string]string{
					"pod1": "sbx1",
					"pod2": "sbx1",
				},
				PodSupplement: 0,
			},
		},
		{
			name: "no need allocated with release",
			spec: &AllocSpec{
				Pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
				Pool: &sandboxv1alpha1.Pool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool1",
					},
				},
				Sandboxes: []*sandboxv1alpha1.BatchSandbox{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "sbx1",
						},
						Spec: sandboxv1alpha1.BatchSandboxSpec{
							PoolRef:  "pool1",
							Replicas: &replica1,
						},
					},
				},
			},
			poolAlloc: &PoolAllocation{
				PodAllocation: map[string]string{
					"pod1": "sbx1",
				},
			},
			sandboxAlloc: &SandboxAllocation{
				Pods: []string{"pod1"},
			},
			release: &AllocationRelease{
				Pods: []string{"pod1"},
			},
			wantStatus: &AllocStatus{
				PodAllocation: map[string]string{},
				PodSupplement: 0,
				DirtyPods:     []string{"pod1"},
			},
		},
	}
	// Recover is called only once due to sync.Once
	store.EXPECT().Recover(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(c.poolAlloc, nil).Times(1)
			store.EXPECT().SetAllocation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			syncer.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(c.sandboxAlloc, nil).Times(len(c.spec.Sandboxes))
			syncer.EXPECT().SetAllocation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			syncer.EXPECT().GetRelease(gomock.Any(), gomock.Any()).Return(c.release, nil).Times(len(c.spec.Sandboxes))
			status, _, _, err := allocator.Schedule(context.Background(), c.spec)
			assert.NoError(t, err)
			if !reflect.DeepEqual(c.wantStatus, status) {
				t.Logf("want: %+v", c.wantStatus)
				t.Logf("got: %+v", status)
			}
			assert.True(t, reflect.DeepEqual(c.wantStatus, status))
		})
	}

}

func TestScheduleReturnsPendingSyncs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := NewMockAllocationStore(ctrl)
	syncer := NewMockAllocationSyncer(ctrl)
	allocator := &defaultAllocator{
		store:  store,
		syncer: syncer,
	}
	replica1 := int32(1)

	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}
	sandboxes := []*sandboxv1alpha1.BatchSandbox{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "sbx1"},
			Spec:       sandboxv1alpha1.BatchSandboxSpec{Replicas: &replica1},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "sbx2"},
			Spec:       sandboxv1alpha1.BatchSandboxSpec{Replicas: &replica1},
		},
	}
	spec := &AllocSpec{
		Pods:      pods,
		Sandboxes: sandboxes,
		Pool:      &sandboxv1alpha1.Pool{ObjectMeta: metav1.ObjectMeta{Name: "pool1"}},
	}

	store.EXPECT().Recover(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(&PoolAllocation{PodAllocation: map[string]string{}}, nil).Times(1)
	syncer.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(&SandboxAllocation{Pods: []string{}}, nil).Times(2)
	syncer.EXPECT().GetRelease(gomock.Any(), gomock.Any()).Return(&AllocationRelease{Pods: []string{}}, nil).Times(2)

	status, pendingSyncs, poolDirty, err := allocator.Schedule(context.Background(), spec)

	assert.NoError(t, err)
	assert.True(t, poolDirty)
	assert.Len(t, pendingSyncs, 2)
	assert.Equal(t, "sbx1", pendingSyncs[0].SandboxName)
	assert.Equal(t, "sbx2", pendingSyncs[1].SandboxName)
	assert.Contains(t, status.PodAllocation, "pod1")
	assert.Contains(t, status.PodAllocation, "pod2")
}

func TestSyncSandboxAllocation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := NewMockAllocationStore(ctrl)
	syncer := NewMockAllocationSyncer(ctrl)
	allocator := &defaultAllocator{
		store:  store,
		syncer: syncer,
	}

	sandbox := &sandboxv1alpha1.BatchSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "sbx1"},
	}
	pods := []string{"pod1", "pod2"}

	syncer.EXPECT().SetAllocation(gomock.Any(), sandbox, gomock.Any()).DoAndReturn(
		func(ctx context.Context, sbx *sandboxv1alpha1.BatchSandbox, alloc *SandboxAllocation) error {
			assert.Equal(t, pods, alloc.Pods)
			return nil
		}).Times(1)

	syncer.EXPECT().GetAllocation(gomock.Any(), sandbox).Return(nil, nil).Times(1)
	err := allocator.SyncSandboxAllocation(context.Background(), sandbox, pods)
	assert.NoError(t, err)
}

func TestDoAllocateIdempotency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := NewMockAllocationStore(ctrl)
	syncer := NewMockAllocationSyncer(ctrl)
	allocator := &defaultAllocator{
		store:  store,
		syncer: syncer,
	}
	replica2 := int32(2)

	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	sandboxes := []*sandboxv1alpha1.BatchSandbox{
		{ObjectMeta: metav1.ObjectMeta{Name: "sbx1"}, Spec: sandboxv1alpha1.BatchSandboxSpec{Replicas: &replica2}},
	}

	poolAlloc := &PoolAllocation{
		PodAllocation: map[string]string{
			"pod1": "sbx1",
		},
	}
	sandboxAlloc := &SandboxAllocation{Pods: []string{}}
	release := &AllocationRelease{Pods: []string{}}

	spec := &AllocSpec{
		Pods:      pods,
		Sandboxes: sandboxes,
		Pool:      &sandboxv1alpha1.Pool{ObjectMeta: metav1.ObjectMeta{Name: "pool1"}},
	}

	store.EXPECT().Recover(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(poolAlloc, nil).Times(1)
	syncer.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(sandboxAlloc, nil).Times(1)
	syncer.EXPECT().GetRelease(gomock.Any(), gomock.Any()).Return(release, nil).Times(1)

	status, pendingSyncs, _, err := allocator.Schedule(context.Background(), spec)

	assert.NoError(t, err)
	assert.Equal(t, "sbx1", status.PodAllocation["pod1"])
	assert.Equal(t, "sbx1", status.PodAllocation["pod2"])
	assert.Len(t, pendingSyncs, 1)
}

func TestDoAllocateSkipsAlreadyAllocatedPod(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := NewMockAllocationStore(ctrl)
	syncer := NewMockAllocationSyncer(ctrl)
	allocator := &defaultAllocator{
		store:  store,
		syncer: syncer,
	}
	replica1 := int32(1)

	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	sandboxes := []*sandboxv1alpha1.BatchSandbox{
		{ObjectMeta: metav1.ObjectMeta{Name: "sbx1"}, Spec: sandboxv1alpha1.BatchSandboxSpec{Replicas: &replica1}},
		{ObjectMeta: metav1.ObjectMeta{Name: "sbx2"}, Spec: sandboxv1alpha1.BatchSandboxSpec{Replicas: &replica1}},
	}

	// Pod1 already allocated to sbx1
	poolAlloc := &PoolAllocation{
		PodAllocation: map[string]string{
			"pod1": "sbx1",
		},
	}

	spec := &AllocSpec{
		Pods:      pods,
		Sandboxes: sandboxes,
		Pool:      &sandboxv1alpha1.Pool{ObjectMeta: metav1.ObjectMeta{Name: "pool1"}},
	}

	store.EXPECT().Recover(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(poolAlloc, nil).Times(1)
	syncer.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(&SandboxAllocation{Pods: []string{"pod1"}}, nil).Times(1)
	syncer.EXPECT().GetAllocation(gomock.Any(), gomock.Any()).Return(&SandboxAllocation{Pods: []string{}}, nil).Times(1)
	syncer.EXPECT().GetRelease(gomock.Any(), gomock.Any()).Return(&AllocationRelease{Pods: []string{}}, nil).Times(2)

	status, _, _, err := allocator.Schedule(context.Background(), spec)

	assert.NoError(t, err)
	// Pod1 should remain with sbx1
	assert.Equal(t, "sbx1", status.PodAllocation["pod1"])
}

func TestSyncSandboxAllocationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := NewMockAllocationStore(ctrl)
	syncer := NewMockAllocationSyncer(ctrl)
	allocator := &defaultAllocator{
		store:  store,
		syncer: syncer,
	}

	sandbox := &sandboxv1alpha1.BatchSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "sbx1"},
	}
	pods := []string{"pod1"}
	oldPods := []string{}

	syncer.EXPECT().GetAllocation(gomock.Any(), sandbox).Return(&SandboxAllocation{Pods: oldPods}, nil).Times(1)
	syncer.EXPECT().SetAllocation(gomock.Any(), sandbox, gomock.Any()).Return(assert.AnError).Times(1)
	store.EXPECT().UpdateAllocation(gomock.Any(), gomock.Any(), "", sandbox.Name, oldPods).Times(1)

	err := allocator.SyncSandboxAllocation(context.Background(), sandbox, pods)
	assert.Error(t, err)
}
