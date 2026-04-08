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
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// BatchSandboxSpec defines the desired state of BatchSandbox.
type BatchSandboxSpec struct {
	// Replicas is the number of desired replicas.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`
	// PoolRef references the Pool resource name for pooled sandbox creation.
	// Mutually exclusive with Template - use PoolRef for pool-based allocation or Template for direct sandbox creation.
	// +optional
	// +kubebuilder:validation:Optional
	PoolRef string `json:"poolRef,omitempty"`
	// +optional
	// Template describes the pods that will be created.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Optional
	Template *corev1.PodTemplateSpec `json:"template"`
	// ShardPatches indicates patching to the Template for BatchSandbox.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	// +kubebuilder:validation:Optional
	ShardPatches []runtime.RawExtension `json:"shardPatches,omitempty"`
	// ExpireTime - Absolute time when the batch-sandbox is deleted.
	// If a time in the past is provided, the batch-sandbox will be deleted immediately.
	// +optional
	// +kubebuilder:validation:Format="date-time"
	// +kubebuilder:validation:Optional
	ExpireTime *metav1.Time `json:"expireTime,omitempty"`
	// Task is a custom task spec that is automatically dispatched after the sandbox is successfully created.
	// The Sandbox is responsible for managing the lifecycle of the task.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Optional
	TaskTemplate *TaskTemplateSpec `json:"taskTemplate,omitempty"`
	// ShardTaskPatches indicates patching to the TaskTemplate for individual Task.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	// +kubebuilder:validation:Optional
	ShardTaskPatches []runtime.RawExtension `json:"shardTaskPatches,omitempty"`
	// TaskResourcePolicyWhenCompleted specifies how resources should be handled once a task reaches a completed state (SUCCEEDED or FAILED).
	// - Retain: Keep the resources until the BatchSandbox is deleted.
	// - Release: Free the resources immediately when the task completes.
	// +optional
	// +kubebuilder:default=Retain
	// +kubebuilder:validation:Optional
	TaskResourcePolicyWhenCompleted *TaskResourcePolicy `json:"taskResourcePolicyWhenCompleted,omitempty"`
	// PausePolicy defines the pause/resume policy for this sandbox.
	// +optional
	PausePolicy *PausePolicy `json:"pausePolicy,omitempty"`
}

type TaskResourcePolicy string

const (
	TaskResourcePolicyRetain  TaskResourcePolicy = "Retain"
	TaskResourcePolicyRelease TaskResourcePolicy = "Release"
)

// PausePolicy defines the policy for pause/resume operations.
type PausePolicy struct {
	// SnapshotRegistry is the OCI registry for snapshot images.
	// +kubebuilder:validation:Required
	SnapshotRegistry string `json:"snapshotRegistry"`

	// SnapshotType indicates the type of snapshot (default: Rootfs).
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Rootfs
	SnapshotType SnapshotType `json:"snapshotType,omitempty"`

	// SnapshotPushSecret is the Secret name for pushing snapshots.
	// +optional
	// +kubebuilder:validation:Optional
	SnapshotPushSecret string `json:"snapshotPushSecret,omitempty"`

	// ResumeImagePullSecret is the Secret name for pulling snapshots during resume.
	// +optional
	// +kubebuilder:validation:Optional
	ResumeImagePullSecret string `json:"resumeImagePullSecret,omitempty"`
}

// BatchSandboxStatus defines the observed state of BatchSandbox.
type BatchSandboxStatus struct {
	// ObservedGeneration is the most recent generation observed for this BatchSandbox. It corresponds to the
	// BatchSandbox's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Replicas is the number of actual Pods
	Replicas int32 `json:"replicas"`
	//	Allocated is the number of actual scheduled Pod
	Allocated int32 `json:"allocated"`
	//	Ready is the number of actual Ready Pod
	Ready int32 `json:"ready"`
	// TaskRunning is the number of Running task
	TaskRunning int32 `json:"taskRunning"`
	// TaskSucceed is the number of Succeed task
	TaskSucceed int32 `json:"taskSucceed"`
	// TaskFailed is the number of Failed task
	TaskFailed int32 `json:"taskFailed"`
	// TaskPending is the number of Pending task which is unassigned
	TaskPending int32 `json:"taskPending"`
	// TaskUnknown is the number of Unknown task
	TaskUnknown int32 `json:"taskUnknown"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bsbx
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".spec.replicas",description="The desired number of pods."
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.replicas",description="The number of currently all pods."
// +kubebuilder:printcolumn:name="ALLOCATED",type="integer",JSONPath=".status.allocated",description="The number of currently all allocated pods."
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.ready",description="The number of currently all ready pods."
// +kubebuilder:printcolumn:name="TASK_RUNNING",type="integer",priority=1,JSONPath=".status.taskRunning",description="The number of currently all running tasks."
// +kubebuilder:printcolumn:name="TASK_SUCCEED",type="integer",priority=1,JSONPath=".status.taskSucceed",description="The number of currently all succeed tasks."
// +kubebuilder:printcolumn:name="TASK_FAILED",type="integer",priority=1,JSONPath=".status.taskFailed",description="The number of currently all failed tasks."
// +kubebuilder:printcolumn:name="TASK_UNKNOWN",type="integer",priority=1,JSONPath=".status.taskUnknown",description="The number of currently all unknown tasks."
// +kubebuilder:printcolumn:name="EXPIRE",type="string",JSONPath=".spec.expireTime",description="sandbox expire time"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// BatchSandbox is the Schema for the batchsandboxes API.
type BatchSandbox struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BatchSandboxSpec   `json:"spec,omitempty"`
	Status BatchSandboxStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BatchSandboxList contains a list of BatchSandbox.
type BatchSandboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BatchSandbox `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BatchSandbox{}, &BatchSandboxList{})
}

// TaskTemplateSpec task spec
type TaskTemplateSpec struct {
	// +optional
	Spec TaskSpec `json:"spec,omitempty"`
}

type TaskSpec struct {
	// +optional
	Process *ProcessTask `json:"process,omitempty"`
	// TimeoutSeconds specifies the maximum duration in seconds for task execution.
	// If exceeded, the task executor should terminate the task.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`
}

type ProcessTask struct {
	// Command command
	// +kubebuilder:validation:Required
	Command []string `json:"command"`
	// Arguments to the entrypoint.
	// +optional
	Args []string `json:"args,omitempty"`
	// List of environment variables to set in the task.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Env []corev1.EnvVar `json:"env,omitempty"`
	// WorkingDir task working directory.
	// +optional
	WorkingDir string `json:"workingDir,omitempty"`
}

// TaskStatus task status
type TaskStatus struct {
	// Details about the task's current condition.
	// +optional
	State TaskState `json:"state,omitempty"`
	// Details about the task's last termination condition.
	// +optional
	LastTerminationState TaskState `json:"lastState,omitempty"`
}

// TaskState holds a possible state of task.
// Only one of its members may be specified.
// If none of them is specified, the default one is TaskStateWaiting.
type TaskState struct {
	// Details about a waiting task
	// +optional
	Waiting *TaskStateWaiting `json:"waiting,omitempty"`
	// Details about a running task
	// +optional
	Running *TaskStateRunning `json:"running,omitempty"`
	// Details about a terminated task
	// +optional
	Terminated *TaskStateTerminated `json:"terminated,omitempty"`
}

// TaskStateWaiting is a waiting state of a task.
type TaskStateWaiting struct {
	// (brief) reason the task is not yet running.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message regarding why the task is not yet running.
	// +optional
	Message string `json:"message,omitempty"`
}

// TaskStateRunning is a running state of a task.
type TaskStateRunning struct {
	// Time at which the task was last (re-)started
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty"`
}

// TaskStateTerminated is a terminated state of a task.
type TaskStateTerminated struct {
	// Exit status from the last termination of the task
	ExitCode int32 `json:"exitCode"`
	// Signal from the last termination of the task
	// +optional
	Signal int32 `json:"signal,omitempty"`
	// (brief) reason from the last termination of the task
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message regarding the last termination of the task
	// +optional
	Message string `json:"message,omitempty"`
	// Time at which previous execution of the task started
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty"`
	// Time at which the task last terminated
	// +optional
	FinishedAt metav1.Time `json:"finishedAt,omitempty"`
}
