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

package proxy

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

// PodWrapper wraps a Pod inside.
type PodWrapper struct{ v1.Pod }

// MakePod creates a Pod wrapper.
func MakePod() *PodWrapper {
	return &PodWrapper{v1.Pod{}}
}

// Obj returns the inner Pod.
func (p *PodWrapper) Obj() *v1.Pod {
	return &p.Pod
}

// Name sets `s` as the name of the inner pod.
func (p *PodWrapper) Name(s string) *PodWrapper {
	p.SetName(s)
	return p
}

// UID sets `s` as the UID of the inner pod.
func (p *PodWrapper) UID(s string) *PodWrapper {
	p.SetUID(types.UID(s))
	return p
}

// SchedulerName sets `s` as the scheduler name of the inner pod.
func (p *PodWrapper) SchedulerName(s string) *PodWrapper {
	p.Spec.SchedulerName = s
	return p
}

// Namespace sets `s` as the namespace of the inner pod.
func (p *PodWrapper) Namespace(s string) *PodWrapper {
	p.SetNamespace(s)
	return p
}

// OwnerReference updates the owning controller of the pod.
func (p *PodWrapper) OwnerReference(name string, gvk schema.GroupVersionKind) *PodWrapper {
	p.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
			Name:       name,
			Controller: pointer.Bool(true),
		},
	}
	return p
}

// Containers sets `containers` to the PodSpec of the inner pod.
func (p *PodWrapper) Containers(containers []v1.Container) *PodWrapper {
	p.Spec.Containers = containers
	return p
}

// Priority sets a priority value into PodSpec of the inner pod.
func (p *PodWrapper) Priority(val int32) *PodWrapper {
	p.Spec.Priority = &val
	return p
}

// CreationTimestamp sets the inner pod's CreationTimestamp.
func (p *PodWrapper) CreationTimestamp(t metav1.Time) *PodWrapper {
	p.ObjectMeta.CreationTimestamp = t
	return p
}

// Terminating sets the inner pod's deletionTimestamp to current timestamp.
func (p *PodWrapper) Terminating() *PodWrapper {
	now := metav1.Now()
	p.DeletionTimestamp = &now
	return p
}

// Node sets `s` as the nodeName of the inner pod.
func (p *PodWrapper) Node(s string) *PodWrapper {
	p.Spec.NodeName = s
	return p
}

// NodeSelector sets `m` as the nodeSelector of the inner pod.
func (p *PodWrapper) NodeSelector(m map[string]string) *PodWrapper {
	p.Spec.NodeSelector = m
	return p
}

// StartTime sets `t` as .status.startTime for the inner pod.
func (p *PodWrapper) StartTime(t metav1.Time) *PodWrapper {
	p.Status.StartTime = &t
	return p
}

// NominatedNodeName sets `n` as the .Status.NominatedNodeName of the inner pod.
func (p *PodWrapper) NominatedNodeName(n string) *PodWrapper {
	p.Status.NominatedNodeName = n
	return p
}

// Phase sets `phase` as .status.Phase of the inner pod.
func (p *PodWrapper) Phase(phase v1.PodPhase) *PodWrapper {
	p.Status.Phase = phase
	return p
}

// IP sets `ip` as .status.PodIP of the inner pod.
func (p *PodWrapper) IP(ip string) *PodWrapper {
	p.Status.PodIP = ip
	return p
}

// Condition adds a `condition(Type, Status, Reason)` to .Status.Conditions.
func (p *PodWrapper) Condition(t v1.PodConditionType, s v1.ConditionStatus, r string) *PodWrapper {
	p.Status.Conditions = append(p.Status.Conditions, v1.PodCondition{Type: t, Status: s, Reason: r})
	return p
}

// Conditions sets `conditions` as .status.Conditions of the inner pod.
func (p *PodWrapper) Conditions(conditions []v1.PodCondition) *PodWrapper {
	p.Status.Conditions = append(p.Status.Conditions, conditions...)
	return p
}

// Toleration creates a toleration (with the operator Exists)
// and injects into the inner pod.
func (p *PodWrapper) Toleration(key string) *PodWrapper {
	p.Spec.Tolerations = append(p.Spec.Tolerations, v1.Toleration{
		Key:      key,
		Operator: v1.TolerationOpExists,
	})
	return p
}

// PVC creates a Volume with a PVC and injects into the inner pod.
func (p *PodWrapper) PVC(name string) *PodWrapper {
	p.Spec.Volumes = append(p.Spec.Volumes, v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: name},
		},
	})
	return p
}

// Volume creates volume and injects into the inner pod.
func (p *PodWrapper) Volume(volume v1.Volume) *PodWrapper {
	p.Spec.Volumes = append(p.Spec.Volumes, volume)
	return p
}

// Volumes set the volumes and inject into the inner pod.
func (p *PodWrapper) Volumes(volumes []v1.Volume) *PodWrapper {
	p.Spec.Volumes = volumes
	return p
}

// Label sets a {k,v} pair to the inner pod label.
func (p *PodWrapper) Label(k, v string) *PodWrapper {
	if p.ObjectMeta.Labels == nil {
		p.ObjectMeta.Labels = make(map[string]string)
	}
	p.ObjectMeta.Labels[k] = v
	return p
}
