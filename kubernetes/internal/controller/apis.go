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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils"
	pkgutils "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/utils"
)

const (
	AnnoAllocStatusKey           = "sandbox.opensandbox.io/alloc-status"
	AnnoAllocReleaseKey          = "sandbox.opensandbox.io/alloc-release"
	LabelBatchSandboxPodIndexKey = "batch-sandbox.sandbox.opensandbox.io/pod-index"

	AnnoPoolAllocStatusKey     = "pool.opensandbox.io/alloc-status"
	AnnoPoolAllocGenerationKey = "pool.opensandbox.io/alloc-generation"

	// Pod Recycle 相关 Annotation
	AnnoPodRecycleMeta = "pool.opensandbox.io/recycle-meta"

	FinalizerTaskCleanup = "batch-sandbox.sandbox.opensandbox.io/task-cleanup"
	FinalizerPoolRecycle = "batch-sandbox.sandbox.opensandbox.io/pool-recycle"

	// Value is the BatchSandbox UID.
	LabelPodDeallocatedFrom = "pool.opensandbox.io/deallocated-from"
	// LabelPodRecycleConfirmed marks that Pool has confirmed recycling.
	// Value is the BatchSandbox UID from deallocated-from label.
	LabelPodRecycleConfirmed = "pool.opensandbox.io/recycle-confirmed"

	AnnoPodRecycleTimeoutSec = "pool.opensandbox.io/recycle-timeout-sec"
)

// PodRecycleState defines the state of Pod recycle.
type PodRecycleState string

const (
	// RecycleStateNone indicates the Pod is in normal state and can be allocated.
	RecycleStateNone PodRecycleState = "None"
	// RecycleStateRestarting indicates the Pod containers are restarting.
	// This is the only active recycle state. The Pod transitions from None → Restarting
	// when a restart is triggered, and back to None when all containers are restarted and ready.
	RecycleStateRestarting PodRecycleState = "Restarting"
)

// PodRecycleMeta holds metadata for Pod recycle state machine.
type PodRecycleMeta struct {
	// State: None or Restarting
	State PodRecycleState `json:"state"`

	// TriggeredAt: Restart trigger timestamp (milliseconds)
	TriggeredAt int64 `json:"triggeredAt"`

	// ContainerRestartCounts stores the restart count of each container before triggering restart.
	// Used to detect if container has restarted even when StartedAt timestamp has second-level precision.
	ContainerRestartCounts map[string]int32 `json:"containerRestartCounts,omitempty"`

	// ContainerStartedAt stores the StartedAt time of each container before triggering restart.
	// Used to detect if container has restarted by comparing StartedAt timestamps.
	ContainerStartedAt map[string]int64 `json:"containerStartedAt,omitempty"`
}

// parsePodRecycleMeta parses the recycle metadata from Pod annotations.
func parsePodRecycleMeta(obj metav1.Object) (*PodRecycleMeta, error) {
	meta := &PodRecycleMeta{}
	if raw := obj.GetAnnotations()[AnnoPodRecycleMeta]; raw != "" {
		if err := json.Unmarshal([]byte(raw), meta); err != nil {
			return nil, err
		}
	}
	return meta, nil
}

// setPodRecycleMeta sets the recycle metadata to Pod annotations.
func setPodRecycleMeta(obj metav1.Object, meta *PodRecycleMeta) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}
	obj.GetAnnotations()[AnnoPodRecycleMeta] = utils.DumpJSON(meta)
}

func isRestarting(pod *corev1.Pod) bool {
	return pod.Annotations[AnnoPodRecycleMeta] != ""
}

func isRecycling(pod *corev1.Pod) bool {
	return pod.Labels[LabelPodDeallocatedFrom] != "" || pod.Annotations[AnnoPodRecycleMeta] != ""
}

// AnnotationSandboxEndpoints Use the exported constant from pkg/utils
var AnnotationSandboxEndpoints = pkgutils.AnnotationEndpoints

type SandboxAllocation struct {
	Pods []string `json:"pods"`
}

type AllocationRelease struct {
	Pods []string `json:"pods"`
}

type PoolAllocation struct {
	PodAllocation map[string]string `json:"podAllocation"`
}

func parseSandboxAllocation(obj metav1.Object) (SandboxAllocation, error) {
	ret := SandboxAllocation{}
	if raw := obj.GetAnnotations()[AnnoAllocStatusKey]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &ret); err != nil {
			return ret, err
		}
	}
	return ret, nil
}

func setSandboxAllocation(obj metav1.Object, alloc SandboxAllocation) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}
	obj.GetAnnotations()[AnnoAllocStatusKey] = utils.DumpJSON(alloc)
}

func parseSandboxReleased(obj metav1.Object) (AllocationRelease, error) {
	ret := AllocationRelease{}
	if raw := obj.GetAnnotations()[AnnoAllocReleaseKey]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &ret); err != nil {
			return ret, err
		}
	}
	return ret, nil
}
