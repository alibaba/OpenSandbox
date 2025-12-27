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

package task_executor

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/api/v1alpha1"
)

// Task represents the internal local task resource (LocalTask)
// It follows the Kubernetes resource model with Metadata, Spec, and Status.
type Task struct {
	Name              string       `json:"name"`
	DeletionTimestamp *metav1.Time `json:"deletionTimestamp,omitempty"`

	// Spec defines the desired behavior of the task.
	// We reuse the v1alpha1.TaskSpec to ensure consistency with the controller API.
	Spec v1alpha1.TaskSpec `json:"spec"`

	// Status describes the current state of the task.
	// We reuse the v1alpha1.TaskStatus to ensure consistency with the controller API.
	Status v1alpha1.TaskStatus `json:"status"`
}
