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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PoolSpec defines the desired state of Pool.
type PoolSpec struct {
	// Pod Template used to create pre-warmed nodes in the pool.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Optional
	Template *corev1.PodTemplateSpec `json:"template"`
	// CapacitySpec controls the size of the resource pool.
	// +kubebuilder:validation:Required
	CapacitySpec CapacitySpec `json:"capacitySpec"`
	// PodRecyclePolicy controls the recycle policy for Pods released from BatchSandbox.
	// +optional
	// +kubebuilder:default=Delete
	PodRecyclePolicy PodRecyclePolicy `json:"podRecyclePolicy,omitempty"`
}

// PodRecyclePolicy defines the recycle policy for Pods released from BatchSandbox.
// +kubebuilder:validation:Enum=Delete;Restart
type PodRecyclePolicy string

const (
	// PodRecyclePolicyDelete deletes the Pod directly when released from BatchSandbox.
	PodRecyclePolicyDelete PodRecyclePolicy = "Delete"
	// PodRecyclePolicyRestart restarts containers before reusing the Pod.
	PodRecyclePolicyRestart PodRecyclePolicy = "Restart"
)

type CapacitySpec struct {
	// BufferMax is the maximum number of nodes kept in the warm buffer.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	BufferMax int32 `json:"bufferMax"`
	// BufferMin is the minimum number of nodes that must remain in the buffer.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	BufferMin int32 `json:"bufferMin"`
	// PoolMax is the maximum total number of nodes allowed in the entire pool.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	PoolMax int32 `json:"poolMax"`
	// PoolMin is the minimum total size of the pool.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	PoolMin int32 `json:"poolMin"`
}

// PoolStatus defines the observed state of Pool.
type PoolStatus struct {
	// ObservedGeneration is the most recent generation observed for this BatchSandbox. It corresponds to the
	// BatchSandbox's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Revision is the latest version of pool
	Revision string `json:"revision"`
	// Total is the total number of nodes in the pool.
	Total int32 `json:"total"`
	// Allocated is the number of nodes currently allocated to sandboxes.
	Allocated int32 `json:"allocated"`
	// Available is the number of nodes currently available in the pool.
	Available int32 `json:"available"`
	// Restarting is the number of Pods that are being restarted for recycle.
	// +optional
	Restarting int32 `json:"restarting,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.total",description="The number of all nodes in pool."
// +kubebuilder:printcolumn:name="ALLOCATED",type="integer",JSONPath=".status.allocated",description="The number of allocated nodes in pool."
// +kubebuilder:printcolumn:name="AVAILABLE",type="integer",JSONPath=".status.available",description="The number of available nodes in pool."
// Pool is the Schema for the pools API.
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PoolSpec   `json:"spec,omitempty"`
	Status PoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PoolList contains a list of Pool.
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pool{}, &PoolList{})
}
