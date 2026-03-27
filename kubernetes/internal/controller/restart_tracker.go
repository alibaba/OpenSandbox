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
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils"
)

// Restart timeout configurations
const (
	defaultRestartTimeout = 90 * time.Second
	killTimeout           = 30 * time.Second
)

// restartTracker manages the Pod restart lifecycle as part of the PoolReconciler.
// It encapsulates all restart-related logic including triggering kills, tracking
// restart progress, and determining when Pods are ready for reuse.
//
// Simplified state machine:
//
//	None → Restarting (trigger kill, fire-and-forget)
//	          ↓ (each reconcile: check final result)
//	    all restarted & ready → None (clear annotation, reuse)
//	    timeout / CrashLoop   → delete Pod
type restartTracker struct {
	client     client.Client
	kubeClient kubernetes.Interface
	restConfig *rest.Config
}

type RestartTracker interface {
	HandleRestart(ctx context.Context, pod *corev1.Pod, timeout time.Duration) error
}

// NewRestartTracker creates a new restartTracker with custom restart timeout.
func NewRestartTracker(c client.Client, kubeClient kubernetes.Interface, restConfig *rest.Config) RestartTracker {
	r := &restartTracker{
		client:     c,
		kubeClient: kubeClient,
		restConfig: restConfig,
	}
	return r
}

// HandleRestart handles the Restart recycle policy for a Pod.
// If the Pod has already been triggered for restart, it checks the restart status.
// Otherwise, it initializes the restart and kicks off a fire-and-forget kill goroutine.
func (t *restartTracker) HandleRestart(ctx context.Context, pod *corev1.Pod, timeout time.Duration) error {
	log := logf.FromContext(ctx)
	// Parse existing meta
	meta, err := parsePodRecycleMeta(pod)
	if err != nil {
		log.Error(err, "Failed to parse recycle meta, will reset and retry", "pod", pod.Name)
		meta = &PodRecycleMeta{}
	}
	// If already triggered, check restart progress
	if meta.TriggeredAt > 0 && meta.State == RecycleStateRestarting {
		return t.checkRestartStatus(ctx, pod, timeout)
	}

	meta.TriggeredAt = time.Now().UnixMilli()
	meta.State = RecycleStateRestarting
	// Record current restart counts to detect restart even when StartedAt has second-level precision
	meta.ContainerRestartCounts = make(map[string]int32)
	meta.ContainerStartedAt = make(map[string]int64)
	for _, container := range pod.Status.ContainerStatuses {
		meta.ContainerRestartCounts[container.Name] = container.RestartCount
		if container.State.Running != nil {
			meta.ContainerStartedAt[container.Name] = container.State.Running.StartedAt.UnixMilli()
		}
	}
	if err = t.updatePodRecycleMeta(ctx, pod, meta); err != nil {
		log.Error(err, "Failed to update recycle meta", "pod", pod.Name)
		return err
	}
	// Fire-and-forget: kill containers in background.
	// This is done after updating the annotation to ensure the restart is tracked.
	t.killPodContainers(ctx, pod)
	log.Info("Triggered restart for Pod", "pod", pod.Name, "triggeredAt", meta.TriggeredAt)
	return nil
}

// updatePodRecycleMeta updates the recycle metadata to Pod annotations and sets the recycle-confirmed label.
// It reads the deallocated-from label value and sets it as recycle-confirmed label.
func (t *restartTracker) updatePodRecycleMeta(ctx context.Context, pod *corev1.Pod, meta *PodRecycleMeta) error {
	old := pod.DeepCopy()
	setPodRecycleMeta(pod, meta)

	// Set recycle-confirmed label from deallocated-from label value
	deallocatedFrom := pod.Labels[LabelPodDeallocatedFrom]
	if deallocatedFrom != "" {
		pod.Labels[LabelPodRecycleConfirmed] = deallocatedFrom
	}

	patch := client.MergeFrom(old)
	return t.client.Patch(ctx, pod, patch)
}

// killPodContainers kills all containers in the Pod (excluding initContainers)
func (t *restartTracker) killPodContainers(ctx context.Context, pod *corev1.Pod) {
	log := logf.FromContext(ctx)
	for _, container := range pod.Spec.Containers {
		go func(cName string) {
			killCtx, cancel := context.WithTimeout(context.Background(), killTimeout)
			defer cancel()

			if err := t.execGracefulKill(killCtx, pod, cName); err != nil {
				log.Info("Graceful kill exec finished with error (may be expected)",
					"pod", pod.Name, "container", cName, "err", err)
			} else {
				log.V(1).Info("Successfully triggered graceful kill", "pod", pod.Name, "container", cName)
			}
		}(container.Name)
	}
}

// execGracefulKill attempts to trigger a SIGTERM (15) signal to the container's PID 1.
func (t *restartTracker) execGracefulKill(ctx context.Context, pod *corev1.Pod, containerName string) error {
	// Common shell entry points in various container images.
	shellEntries := []string{"/bin/sh", "/usr/bin/sh", "sh"}

	var lastErr error
	for _, entry := range shellEntries {
		cmd := []string{
			entry, "-c",
			"if [ -x /bin/kill ]; then /bin/kill -15 1; " +
				"elif [ -x /usr/bin/kill ]; then /usr/bin/kill -15 1; " +
				"else kill -15 1; fi",
		}
		err := t.executeExec(ctx, pod, containerName, cmd)
		if err == nil {
			return nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "executable file not found") &&
			!strings.Contains(err.Error(), "no such file or directory") {
			break
		}
	}
	return lastErr
}

// executeExec performs a low-level Pod exec operation.
func (t *restartTracker) executeExec(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string) error {
	req := t.kubeClient.CoreV1().RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(t.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
}

// checkRestartStatus checks if the Pod has completed restart and is ready to be reused.
func (t *restartTracker) checkRestartStatus(ctx context.Context, pod *corev1.Pod, timeout time.Duration) error {
	log := logf.FromContext(ctx)

	meta, err := parsePodRecycleMeta(pod)
	if err != nil {
		log.Error(err, "Failed to parse recycle meta", "pod", pod.Name)
		return err
	}

	elapsed := time.Duration(time.Now().UnixMilli()-meta.TriggeredAt) * time.Millisecond

	allRestarted := true
	triggerAt := time.UnixMilli(meta.TriggeredAt)
	for _, container := range pod.Status.ContainerStatuses {
		restarted := false
		running := container.State.Running
		// Check if container has restarted by either:
		// 1. StartedAt time is after trigger time (original logic)
		// 2. RestartCount has increased (handles same-second restarts)
		// 3. StartedAt time is greater than recorded original StartedAt
		if running != nil && running.StartedAt.Time.After(triggerAt) {
			restarted = true
			log.Info("Container restarted detected by start time after trigger",
				"pod", pod.Name, "container", container.Name,
				"trigger", triggerAt, "current", running.StartedAt.Time)
		} else if originalCount, ok := meta.ContainerRestartCounts[container.Name]; ok && container.RestartCount > originalCount {
			restarted = true
			log.Info("Container restarted detected by restart count increased",
				"pod", pod.Name, "container", container.Name,
				"originalCount", originalCount, "currentCount", container.RestartCount)
		} else if running != nil {
			if originalStartedAt, ok := meta.ContainerStartedAt[container.Name]; ok {
				if running.StartedAt.UnixMilli() > originalStartedAt {
					restarted = true
					log.Info("Container restarted detected by startedAt increased",
						"pod", pod.Name, "container", container.Name,
						"originalStartedAt", time.UnixMilli(originalStartedAt), "currentStartedAt", running.StartedAt.Time)
				}
			}
		}
		if !restarted || !container.Ready {
			allRestarted = false
		}
	}

	podReady := utils.IsPodReady(pod)
	if allRestarted && podReady {
		if err = t.clearPodRecycleMeta(ctx, pod); err != nil {
			return err
		}
		log.Info("Pod restart completed, ready for reuse", "pod", pod.Name, "elapsed", elapsed)
		// Trigger requeue to ensure subsequent checks see the updated pod state.
		// This prevents race conditions where another reconcile reads stale cached data.
	}
	restartTimeout := timeout
	if restartTimeout == 0 {
		restartTimeout = defaultRestartTimeout
	}
	if elapsed > restartTimeout {
		log.Info("Pod restart timeout, deleting", "pod", pod.Name,
			"elapsed", elapsed, "timeout", restartTimeout,
			"allRestarted", allRestarted)
		return t.client.Delete(ctx, pod)
	}
	log.Info("Pod still restarting", "pod", pod.Name, "elapsed", elapsed,
		"allRestarted", allRestarted, "podReady", podReady, "timeout", restartTimeout, "elapsed", elapsed)
	return nil
}

// clearPodRecycleMeta clears the recycle metadata annotation from Pod and the deallocated-from label.
// It keeps the recycle-confirmed label as a receipt that recycling was processed.
// After successful patch, it re-fetches the pod to ensure the local object reflects the latest state.
func (t *restartTracker) clearPodRecycleMeta(ctx context.Context, pod *corev1.Pod) error {
	old := pod.DeepCopy()
	if pod.Annotations != nil {
		delete(pod.Annotations, AnnoPodRecycleMeta)
	}
	if pod.Labels != nil {
		delete(pod.Labels, LabelPodDeallocatedFrom)
	}

	patch := client.MergeFrom(old)
	if err := t.client.Patch(ctx, pod, patch); err != nil {
		return err
	}
	return nil
}
