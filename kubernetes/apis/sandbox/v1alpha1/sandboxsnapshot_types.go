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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// SnapshotType defines the type of snapshot.
type SnapshotType string

const (
	SnapshotTypeRootfs SnapshotType = "Rootfs"
)

// +kubebuilder:validation:Enum=Pending;Committing;Ready;Failed
// SandboxSnapshotPhase defines the phase of a snapshot.
type SandboxSnapshotPhase string

const (
	SandboxSnapshotPhasePending    SandboxSnapshotPhase = "Pending"
	SandboxSnapshotPhaseCommitting SandboxSnapshotPhase = "Committing"
	SandboxSnapshotPhaseReady      SandboxSnapshotPhase = "Ready"
	SandboxSnapshotPhaseFailed     SandboxSnapshotPhase = "Failed"
)

// ContainerSnapshot represents a snapshot of a single container.
type ContainerSnapshot struct {
	// ContainerName is the name of the container.
	ContainerName string `json:"containerName"`
	// ImageURI is the target image URI for this container's snapshot.
	ImageURI string `json:"imageUri"`
	// ImageDigest is the digest of the pushed snapshot image.
	// +optional
	ImageDigest string `json:"imageDigest,omitempty"`
}

// SandboxSnapshotSpec defines the desired state of SandboxSnapshot.
type SandboxSnapshotSpec struct {
	// SandboxID is the stable public identifier for the sandbox.
	SandboxID string `json:"sandboxId"`

	// SnapshotType indicates the type of snapshot (default: Rootfs).
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Rootfs
	SnapshotType SnapshotType `json:"snapshotType,omitempty"`

	// SourceBatchSandboxName is the name of the source BatchSandbox.
	SourceBatchSandboxName string `json:"sourceBatchSandboxName"`

	// SourcePodName is the name of the source Pod.
	// +optional
	// +kubebuilder:validation:Optional
	SourcePodName string `json:"sourcePodName,omitempty"`

	// SourceNodeName is the node where the source Pod runs.
	// +optional
	// +kubebuilder:validation:Optional
	SourceNodeName string `json:"sourceNodeName,omitempty"`

	// SnapshotPushSecret is the Secret name for pushing to registry.
	// +optional
	SnapshotPushSecret string `json:"snapshotPushSecret,omitempty"`

	// ResumeImagePullSecret is the Secret name for pulling snapshot during resume.
	// +optional
	ResumeImagePullSecret string `json:"resumeImagePullSecret,omitempty"`

	// ResumeTemplate contains enough information to reconstruct BatchSandbox.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ResumeTemplate *runtime.RawExtension `json:"resumeTemplate,omitempty"`

	// SnapshotRegistry is the OCI registry for snapshot images.
	// +optional
	// +kubebuilder:validation:Optional
	SnapshotRegistry string `json:"snapshotRegistry,omitempty"`

	// ContainerSnapshots holds per-container snapshot information.
	// The controller fills this during resolution.
	// +optional
	// +kubebuilder:validation:Optional
	ContainerSnapshots []ContainerSnapshot `json:"containerSnapshots,omitempty"`

	// PausedAt is the timestamp when pause was initiated.
	PausedAt metav1.Time `json:"pausedAt"`

	// PauseVersion is incremented by the server to request a pause.
	// Controller ACKs by setting status.pauseVersion to match when entering Committing phase.
	PauseVersion int `json:"pauseVersion"`

	// ResumeVersion is incremented by the server to request a resume.
	// Controller ACKs by setting status.resumeVersion to match when starting resume.
	ResumeVersion int `json:"resumeVersion"`
}

// SnapshotRecord represents a single pause or resume event in the snapshot history.
type SnapshotRecord struct {
	// Action is "Pause" or "Resume".
	Action string `json:"action"`
	// Version is the pauseVersion or resumeVersion that triggered this action.
	Version int `json:"version"`
	// Timestamp is when this record was created.
	Timestamp metav1.Time `json:"timestamp"`
	// Message is a human-readable description of the event.
	Message string `json:"message"`
}

// SandboxSnapshotStatus defines the observed state of SandboxSnapshot.
type SandboxSnapshotStatus struct {
	// Phase indicates the current phase of the snapshot.
	Phase SandboxSnapshotPhase `json:"phase,omitempty"`

	// Message provides human-readable status information.
	Message string `json:"message,omitempty"`

	// ReadyAt is the timestamp when the snapshot became Ready.
	ReadyAt *metav1.Time `json:"readyAt,omitempty"`

	// ContainerSnapshots holds per-container snapshot results (filled by controller after push).
	// +optional
	ContainerSnapshots []ContainerSnapshot `json:"containerSnapshots,omitempty"`

	// PauseVersion is ACKed by the controller when entering Committing phase.
	PauseVersion int `json:"pauseVersion"`

	// ResumeVersion is ACKed by the controller when starting resume.
	ResumeVersion int `json:"resumeVersion"`

	// History records each pause/resume cycle.
	// +optional
	History []SnapshotRecord `json:"history,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sbxsnap
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="SANDBOX_ID",type="string",JSONPath=".spec.sandboxId"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type SandboxSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxSnapshotSpec   `json:"spec,omitempty"`
	Status SandboxSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SandboxSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SandboxSnapshot{}, &SandboxSnapshotList{})
}
