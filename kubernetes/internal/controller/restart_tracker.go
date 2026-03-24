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
	restartTimeout = 90 * time.Second
	killTimeout    = 30 * time.Second
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
	HandleRestart(ctx context.Context, pod *corev1.Pod) error
}

// NewRestartTracker creates a new restartTracker.
func NewRestartTracker(c client.Client, kubeClient kubernetes.Interface, restConfig *rest.Config) RestartTracker {
	return &restartTracker{
		client:     c,
		kubeClient: kubeClient,
		restConfig: restConfig,
	}
}

// HandleRestart handles the Restart recycle policy for a Pod.
// If the Pod has already been triggered for restart, it checks the restart status.
// Otherwise, it initializes the restart and kicks off a fire-and-forget kill goroutine.
func (t *restartTracker) HandleRestart(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)
	// Parse existing meta
	meta, err := parsePodRecycleMeta(pod)
	if err != nil {
		log.Error(err, "Failed to parse recycle meta, will reset and retry", "pod", pod.Name)
		meta = &PodRecycleMeta{}
	}
	// If already triggered, check restart progress
	if meta.TriggeredAt > 0 && meta.State == RecycleStateRestarting {
		return t.checkRestartStatus(ctx, pod)
	}

	meta.TriggeredAt = time.Now().UnixMilli()
	meta.State = RecycleStateRestarting
	meta.InitialRestartCounts = make(map[string]int32)
	for _, container := range pod.Status.ContainerStatuses {
		meta.InitialRestartCounts[container.Name] = container.RestartCount
	}
	if err = t.updatePodRecycleMeta(ctx, pod, meta); err != nil {
		log.Error(err, "Failed to update recycle meta", "pod", pod.Name)
		return err
	}
	// Fire-and-forget: kill containers in background.
	t.killPodContainers(ctx, pod)
	log.Info("Triggered restart for Pod", "pod", pod.Name)
	return nil
}

// killPodContainers kills all containers in the Pod (excluding initContainers)
func (t *restartTracker) killPodContainers(ctx context.Context, pod *corev1.Pod) {
	log := logf.FromContext(ctx)
	for _, container := range pod.Spec.Containers {
		go func(cName string, ctx context.Context) {
			killCtx, cancel := context.WithTimeout(ctx, killTimeout)
			defer cancel()

			if err := t.execGracefulKill(killCtx, pod, cName); err != nil {
				log.Info("Graceful kill exec finished with error (may be expected)",
					"pod", pod.Name, "container", cName, "err", err)
			} else {
				log.V(1).Info("Successfully triggered graceful kill", "pod", pod.Name, "container", cName)
			}
		}(container.Name, ctx)
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
func (t *restartTracker) checkRestartStatus(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	meta, err := parsePodRecycleMeta(pod)
	if err != nil {
		log.Error(err, "Failed to parse recycle meta", "pod", pod.Name)
		return err
	}

	elapsed := time.Duration(time.Now().UnixMilli()-meta.TriggeredAt) * time.Millisecond

	allRestarted := true
	allReady := true
	for _, container := range pod.Status.ContainerStatuses {
		initialCount, exists := meta.InitialRestartCounts[container.Name]
		if !exists || container.RestartCount <= initialCount {
			allRestarted = false
		}
		if !container.Ready {
			allReady = false
		}
	}

	podReady := utils.IsPodReady(pod)
	if allRestarted && allReady && podReady {
		if err := t.clearPodRecycleMeta(ctx, pod); err != nil {
			return err
		}
		log.Info("Pod restart completed, ready for reuse", "pod", pod.Name, "elapsed", elapsed)
		return nil
	}

	if isCrashLoopBackOff(pod) {
		log.Info("Pod entered CrashLoopBackOff during restart, deleting", "pod", pod.Name)
		return t.client.Delete(ctx, pod)
	}

	if elapsed > restartTimeout {
		log.Info("Pod restart timeout, deleting", "pod", pod.Name,
			"elapsed", elapsed, "timeout", restartTimeout,
			"allRestarted", allRestarted, "allReady", allReady)
		return t.client.Delete(ctx, pod)
	}

	log.V(1).Info("Pod still restarting", "pod", pod.Name, "elapsed", elapsed,
		"allRestarted", allRestarted, "allReady", allReady, "podReady", podReady)
	return nil
}

// updatePodRecycleMeta updates the recycle metadata to Pod annotations.
func (t *restartTracker) updatePodRecycleMeta(ctx context.Context, pod *corev1.Pod, meta *PodRecycleMeta) error {
	old := pod.DeepCopy()
	setPodRecycleMeta(pod, meta)
	patch := client.MergeFrom(old)
	return t.client.Patch(ctx, pod, patch)
}

// isCrashLoopBackOff checks if the Pod is in CrashLoopBackOff state.
func isCrashLoopBackOff(pod *corev1.Pod) bool {
	for _, container := range pod.Status.ContainerStatuses {
		if container.State.Waiting != nil {
			if container.State.Waiting.Reason == "CrashLoopBackOff" {
				return true
			}
		}
	}
	return false
}

// clearPodRecycleMeta clears the recycle metadata annotation from Pod.
func (t *restartTracker) clearPodRecycleMeta(ctx context.Context, pod *corev1.Pod) error {
	old := pod.DeepCopy()
	anno := pod.GetAnnotations()
	if anno != nil {
		delete(anno, AnnoPodRecycleMeta)
	}
	patch := client.MergeFrom(old)
	return t.client.Patch(ctx, pod, patch)
}
