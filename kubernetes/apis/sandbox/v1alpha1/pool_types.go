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
	// PodRecyclePolicy specifies how to handle allocated Pods when a pooled BatchSandbox is deleted.
	// - Delete: (default) Delete the allocated Pod directly.
	// - Reuse: Reset the Pod before returning it to the pool; if reset fails, delete it.
	// Note: Reuse policy requires task-executor image to be configured in controller.
	// If not configured, pods will be deleted with a warning log.
	// +optional
	// +kubebuilder:default=Delete
	// +kubebuilder:validation:Enum=Delete;Reuse
	PodRecyclePolicy PodRecyclePolicy `json:"podRecyclePolicy,omitempty"`
	// ResetSpec specifies the reset configuration when PodRecyclePolicy is Reuse.
	// Ignored when PodRecyclePolicy is Delete.
	// +optional
	ResetSpec *ResetSpec `json:"resetSpec,omitempty"`
}

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

// PodRecyclePolicy defines the policy for recycling pooled Pods.
type PodRecyclePolicy string

const (
	// PodRecyclePolicyDelete deletes the allocated Pod directly.
	// This is the default behavior.
	PodRecyclePolicyDelete PodRecyclePolicy = "Delete"
	// PodRecyclePolicyReuse resets the Pod before returning it to the pool.
	// Requires task-executor image to be configured in controller.
	PodRecyclePolicyReuse PodRecyclePolicy = "Reuse"
)

// ResetSpec specifies how to reset a Pod before returning it to the pool.
type ResetSpec struct {
	// MainContainerName specifies which container is the main container for reset purposes.
	// The main container will be restarted during reset.
	// If not specified, the first container in the pod template is used.
	// +optional
	MainContainerName string `json:"mainContainerName,omitempty"`
	// CleanDirectories specifies directories to clean during reset.
	// Supports glob patterns like "/tmp/*", "/var/cache/**".
	// Default: ["/tmp"]
	// +optional
	CleanDirectories []string `json:"cleanDirectories,omitempty"`
	// TimeoutSeconds specifies the timeout for reset operation in seconds.
	// +optional
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=600
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`
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
	// Resetting is the number of Pods currently being reset.
	// +optional
	Resetting int32 `json:"resetting,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.total",description="The number of all nodes in pool."
// +kubebuilder:printcolumn:name="ALLOCATED",type="integer",JSONPath=".status.allocated",description="The number of allocated nodes in pool."
// +kubebuilder:printcolumn:name="AVAILABLE",type="integer",JSONPath=".status.available",description="The number of available nodes in pool."
// +kubebuilder:printcolumn:name="RESETTING",type="integer",JSONPath=".status.resetting",description="The number of pods being reset in pool."
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
