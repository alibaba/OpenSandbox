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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils"
)

const (
	// SandboxSnapshotFinalizer is the finalizer for SandboxSnapshot cleanup
	SandboxSnapshotFinalizer = "sandboxsnapshot.sandbox.opensandbox.io/cleanup"

	// DefaultCommitJobTimeout is the default timeout for commit jobs
	DefaultCommitJobTimeout = 10 * time.Minute

	DefaultTTLSecondsAfterFinished = 300

	// CommitJobContainerName is the container name in commit job
	CommitJobContainerName = "commit"

	// ContainerdSocketPath is the default containerd socket path
	ContainerdSocketPath = "/var/run/containerd/containerd.sock"

	// LabelSandboxSnapshotName is the label key for sandbox snapshot name
	LabelSandboxSnapshotName = "sandbox.opensandbox.io/sandbox-snapshot-name"

	// AnnotationResumedFromSnapshot marks a BatchSandbox as resumed from a snapshot
	AnnotationResumedFromSnapshot = "sandbox.opensandbox.io/resumed-from-snapshot"

	// MaxHistoryRecords is the maximum number of history records to keep
	MaxHistoryRecords = 10
)

// SandboxSnapshotReconciler reconciles a SandboxSnapshot object
type SandboxSnapshotReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// ImageCommitterImage is the image for image-committer (uses nerdctl to commit/push container images)
	ImageCommitterImage string

	// CommitJobTimeout is the timeout for commit jobs (default: 10 minutes)
	CommitJobTimeout time.Duration
}

// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=sandboxsnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=sandboxsnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=sandboxsnapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=batchsandboxes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=pools,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *SandboxSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the SandboxSnapshot instance
	snapshot := &sandboxv1alpha1.SandboxSnapshot{}
	if err := r.Get(ctx, req.NamespacedName, snapshot); err != nil {
		if errors.IsNotFound(err) {
			log.Info("SandboxSnapshot resource not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get SandboxSnapshot")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !snapshot.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, snapshot)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(snapshot, SandboxSnapshotFinalizer) {
		if err := utils.UpdateFinalizer(r.Client, snapshot, utils.AddFinalizerOpType, SandboxSnapshotFinalizer); err != nil {
			log.Error(err, "Failed to add finalizer", "finalizer", SandboxSnapshotFinalizer)
			return ctrl.Result{}, err
		}
		log.Info("Added finalizer", "finalizer", SandboxSnapshotFinalizer)
		return ctrl.Result{RequeueAfter: time.Millisecond * 100}, nil
	}

	// Generation-driven dispatch
	generation := snapshot.Generation
	observedGen := snapshot.Status.ObservedGeneration

	log.Info("Reconciling SandboxSnapshot",
		"snapshot", snapshot.Name,
		"phase", snapshot.Status.Phase,
		"generation", generation, "observedGeneration", observedGen,
		"action", snapshot.Spec.Action,
	)

	// 1. Resume requested: action == "Resume" with new generation
	if snapshot.Spec.Action == sandboxv1alpha1.SnapshotActionResume && generation > observedGen {
		return r.handleResume(ctx, snapshot)
	}

	// 2. New pause requested: action == "Pause" (or unset) with new generation
	if generation > observedGen {
		phase := snapshot.Status.Phase
		if phase == "" || phase == sandboxv1alpha1.SandboxSnapshotPhaseReady || phase == sandboxv1alpha1.SandboxSnapshotPhaseFailed {
			isValid, validateErr := r.validatePauseSpec(ctx, snapshot)
			if !isValid {
				log.Error(validateErr, "Spec validation failed, transitioning to Failed")
				r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "InvalidSpec", "Spec validation failed: %v", validateErr)
				return ctrl.Result{}, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
					latest := &sandboxv1alpha1.SandboxSnapshot{}
					if err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: snapshot.Name}, latest); err != nil {
						return err
					}
					latest.Status.Phase = sandboxv1alpha1.SandboxSnapshotPhaseFailed
					latest.Status.Message = validateErr.Error()
					latest.Status.ObservedGeneration = latest.Generation
					return r.Status().Update(ctx, latest)
				})
			}
			if validateErr != nil {
				// Transient API error (e.g. apiserver unreachable); requeue to retry.
				log.Error(validateErr, "Transient error during spec validation, requeueing")
				return ctrl.Result{}, validateErr
			}
			return r.startNewPauseCycle(ctx, snapshot)
		}
		// Continue current pause cycle
		switch phase {
		case sandboxv1alpha1.SandboxSnapshotPhasePending:
			return r.handlePending(ctx, snapshot)
		case sandboxv1alpha1.SandboxSnapshotPhaseCommitting:
			return r.handleCommitting(ctx, snapshot)
		default:
			log.Info("Unexpected phase during pause, treating as Pending", "phase", phase)
			return r.handlePending(ctx, snapshot)
		}
	}

	// 3. Idle — generation matches, dispatch by phase for ongoing work
	phase := snapshot.Status.Phase
	switch phase {
	case sandboxv1alpha1.SandboxSnapshotPhasePending:
		return r.handlePending(ctx, snapshot)
	case sandboxv1alpha1.SandboxSnapshotPhaseCommitting:
		return r.handleCommitting(ctx, snapshot)
	case sandboxv1alpha1.SandboxSnapshotPhaseReady:
		return r.handleReady(ctx, snapshot)
	case sandboxv1alpha1.SandboxSnapshotPhaseFailed:
		return r.handleFailed(ctx, snapshot)
	default:
		log.Info("Idle with no pending work", "phase", phase)
		return ctrl.Result{}, nil
	}
}

// startNewPauseCycle initializes a new pause cycle by incrementing the internal pause counter
// and resetting the phase to Pending.
// It also clears SourcePodName and SourceNodeName to force re-resolution for each pause cycle.
func (r *SandboxSnapshotReconciler) startNewPauseCycle(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Increment internal pause counter
	newPauseVersion := snapshot.Status.PauseVersion + 1
	now := metav1.Now()
	log.Info("Starting new pause cycle",
		"pauseVersion", newPauseVersion,
		"generation", snapshot.Generation,
	)

	// Reset to Pending phase with incremented PauseVersion and LastPauseAt
	// Also clear SourcePodName and SourceNodeName to force re-resolution
	// This is important when resuming from a Pool-based sandbox:
	// - First pause: uses pool pod (e.g., pool-xxx)
	// - Resume: creates a new BatchSandbox (non-pool mode)
	// - Second pause: should use the new pod (e.g., sandbox-id-0), not the old pool pod
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latestSnapshot := &sandboxv1alpha1.SandboxSnapshot{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: snapshot.Name}, latestSnapshot); err != nil {
			return err
		}
		latestSnapshot.Status.Phase = sandboxv1alpha1.SandboxSnapshotPhasePending
		latestSnapshot.Status.Message = "Pause requested"
		latestSnapshot.Status.PauseVersion = newPauseVersion
		latestSnapshot.Status.LastPauseAt = &now
		// Clear source pod info to force re-resolution for this pause cycle
		latestSnapshot.Status.SourcePodName = ""
		latestSnapshot.Status.SourceNodeName = ""
		return r.Status().Update(ctx, latestSnapshot)
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Millisecond * 100}, nil
}

// resolvedSnapshotData holds the resolved data for a snapshot.
// This is used internally by the controller and not persisted in spec.
type resolvedSnapshotData struct {
	containerSnapshots    []sandboxv1alpha1.ContainerSnapshot
	sourcePodName         string
	sourceNodeName        string
	snapshotType          sandboxv1alpha1.SnapshotType
	snapshotRegistry      string
	snapshotPushSecret    string
	resumeImagePullSecret string
	resumeTemplate        *runtime.RawExtension
}

// ensureResolved resolves the template and returns container snapshots info.
// It no longer modifies spec.ContainerSnapshots - the caller is responsible
// for persisting the resolved data to status.
func (r *SandboxSnapshotReconciler) ensureResolved(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (*resolvedSnapshotData, error) {
	log := logf.FromContext(ctx)

	// If status.ContainerSnapshots already populated, re-generate image URIs
	// with current pauseVersion (they may be stale from a previous pause cycle).
	if len(snapshot.Status.ContainerSnapshots) > 0 {
		allResolved := true
		for _, cs := range snapshot.Status.ContainerSnapshots {
			if cs.ContainerName != "" && cs.ImageURI != "" {
				continue
			}
			allResolved = false
			break
		}

		// Check also if essential pause policy fields are populated in spec
		// AND sourcePodName is already populated (otherwise we need to re-resolve)
		if allResolved && snapshot.Spec.SnapshotType != "" && snapshot.Spec.SnapshotRegistry != "" && snapshot.Status.SourcePodName != "" {
			// Re-generate image URIs to reflect current pauseVersion
			registry := snapshot.Spec.SnapshotRegistry
			containerSnapshots := make([]sandboxv1alpha1.ContainerSnapshot, len(snapshot.Status.ContainerSnapshots))
			for i, cs := range snapshot.Status.ContainerSnapshots {
				expectedURI := fmt.Sprintf("%s/%s-%s:snapshot-v%d", registry, snapshot.Spec.SandboxID, cs.ContainerName, snapshot.Status.PauseVersion)
				if cs.ImageURI != expectedURI {
					log.Info("Updating stale image URI for re-pause", "container", cs.ContainerName, "old", cs.ImageURI, "new", expectedURI)
					containerSnapshots[i] = sandboxv1alpha1.ContainerSnapshot{
						ContainerName: cs.ContainerName,
						ImageURI:      expectedURI,
					}
				} else {
					containerSnapshots[i] = cs
				}
			}
			log.Info("Snapshot already resolved, returning updated container snapshots")
			return &resolvedSnapshotData{
				containerSnapshots: containerSnapshots,
				sourcePodName:      snapshot.Status.SourcePodName,
				sourceNodeName:     snapshot.Status.SourceNodeName,
			}, nil
		}
	}

	// Look up the source BatchSandbox
	bs := &sandboxv1alpha1.BatchSandbox{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      snapshot.Spec.SourceBatchSandboxName,
		Namespace: snapshot.Namespace,
	}, bs); err != nil {
		return nil, fmt.Errorf("failed to get source BatchSandbox %s: %w", snapshot.Spec.SourceBatchSandboxName, err)
	}

	data := &resolvedSnapshotData{}

	// If SourcePodName is empty in status, find the running pod for this sandbox
	if snapshot.Status.SourcePodName == "" {
		pod, err := r.findPodForSandbox(ctx, bs, snapshot.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to find running pod for sandbox: %w", err)
		}
		data.sourcePodName = pod.Name
		data.sourceNodeName = pod.Spec.NodeName
		log.Info("Resolved pod info", "pod", pod.Name, "node", pod.Spec.NodeName)
	}

	// Read pause configuration from snapshot.Spec (filled by Server from config)
	// No longer read from BatchSandbox.PausePolicy (field removed from CRD)
	if snapshot.Spec.SnapshotRegistry == "" {
		return nil, fmt.Errorf("snapshotRegistry not configured in snapshot spec")
	}
	data.snapshotRegistry = snapshot.Spec.SnapshotRegistry
	data.snapshotType = snapshot.Spec.SnapshotType
	data.snapshotPushSecret = snapshot.Spec.SnapshotPushSecret
	data.resumeImagePullSecret = snapshot.Spec.ResumeImagePullSecret

	// Resolve the template: prefer spec.Template, otherwise look up Pool CR
	var template *corev1.PodTemplateSpec
	if bs.Spec.Template != nil {
		template = bs.Spec.Template
		log.Info("Resolved template directly from BatchSandbox spec")
	} else if bs.Spec.PoolRef != "" {
		// PoolRef mode: look up the Pool CR to get template
		pool := &sandboxv1alpha1.Pool{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      bs.Spec.PoolRef,
			Namespace: snapshot.Namespace,
		}, pool); err != nil {
			return nil, fmt.Errorf("failed to look up Pool CR %s to get template: %w", bs.Spec.PoolRef, err)
		}
		if pool.Spec.Template == nil {
			return nil, fmt.Errorf("Pool %s has no template defined", bs.Spec.PoolRef)
		}
		template = pool.Spec.Template
		log.Info("Resolved template via Pool CR", "pool", bs.Spec.PoolRef)
	} else {
		return nil, fmt.Errorf("BatchSandbox %s has neither template nor poolRef, cannot resolve", bs.Name)
	}

	// Build ResumeTemplate from the template with resolved fields
	// Note: pausePolicy is NOT included - pause config comes from server config, not from BatchSandbox
	resumeTemplateData := map[string]interface{}{
		"template": convertPodTemplateSpecToMap(template),
	}

	// Add BatchSandbox-level fields to ResumeTemplate if they exist
	if bs.Spec.ExpireTime != nil {
		resumeTemplateData["expireTime"] = bs.Spec.ExpireTime
	}

	// Preserve user-defined labels from BatchSandbox metadata so they survive the
	userLabels := make(map[string]string)
	for k, v := range bs.Labels {
		if strings.HasPrefix(k, "opensandbox.io/") || strings.HasPrefix(k, "sandbox.opensandbox.io/") {
			continue
		}
		userLabels[k] = v
	}
	if len(userLabels) > 0 {
		resumeTemplateData["metadata"] = map[string]interface{}{
			"labels": userLabels,
		}
	}

	// Convert the entire resume template to RawExtension
	resumeTemplateRaw, err := convertToRawExtension(resumeTemplateData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resume template to raw extension: %w", err)
	}
	data.resumeTemplate = &resumeTemplateRaw

	// Resolve snapshot registry
	registry := data.snapshotRegistry
	if registry == "" {
		return nil, fmt.Errorf("snapshotRegistry not resolved in pausePolicy")
	}

	// Build ContainerSnapshots from the template containers
	containerSnapshots := make([]sandboxv1alpha1.ContainerSnapshot, 0, len(template.Spec.Containers))
	for _, c := range template.Spec.Containers {
		// Include pauseVersion in image tag to distinguish between multiple pauses
		imageURI := fmt.Sprintf("%s/%s-%s:snapshot-v%d", registry, snapshot.Spec.SandboxID, c.Name, snapshot.Status.PauseVersion)
		containerSnapshots = append(containerSnapshots, sandboxv1alpha1.ContainerSnapshot{
			ContainerName: c.Name,
			ImageURI:      imageURI,
		})
	}

	if len(containerSnapshots) == 0 {
		return nil, fmt.Errorf("no containers found in template for BatchSandbox %s", bs.Name)
	}

	data.containerSnapshots = containerSnapshots
	log.Info("Resolved snapshot fields", "count", len(containerSnapshots), "snapshot", snapshot.Name)
	return data, nil
}

// findPodForSandbox finds the running pod belonging to a BatchSandbox.
// It first tries to parse the alloc-status annotation, then falls back to label selector.
func (r *SandboxSnapshotReconciler) findPodForSandbox(ctx context.Context, bs *sandboxv1alpha1.BatchSandbox, namespace string) (*corev1.Pod, error) {
	log := logf.FromContext(ctx)

	// Try alloc-status annotation first (pool-based allocation)
	alloc, err := parseSandboxAllocation(bs)
	if err == nil && len(alloc.Pods) > 0 {
		// Get the first allocated pod
		pod := &corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: alloc.Pods[0]}, pod); err == nil {
			if pod.Status.Phase == corev1.PodRunning {
				return pod, nil
			}
			log.Info("Allocated pod not running, trying others", "pod", pod.Name, "phase", pod.Status.Phase)
		}
		// Try other pods in the allocation
		for _, podName := range alloc.Pods[1:] {
			p := &corev1.Pod{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, p); err == nil {
				if p.Status.Phase == corev1.PodRunning {
					return p, nil
				}
			}
		}
	}

	// Fallback 1: find by batch-sandbox name label (efficient label selector)
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{LabelBatchSandboxNameKey: bs.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}

	// Fallback 2: find by naming convention {batchSandboxName}-0
	podName := fmt.Sprintf("%s-0", bs.Name)
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod); err == nil {
		if pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}

	return nil, fmt.Errorf("no running pod found for BatchSandbox %s", bs.Name)
}

// persistResolvedData persists the resolved data to status only.
// Per pause-policy-refactor.md design, Controller never writes to spec to avoid generation increments.
// All resolved fields (SourcePodName, SourceNodeName, ResumeTemplate, ContainerSnapshots) go to status.
func (r *SandboxSnapshotReconciler) persistResolvedData(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot, data *resolvedSnapshotData) error {
	log := logf.FromContext(ctx)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latestSnapshot := &sandboxv1alpha1.SandboxSnapshot{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: snapshot.Name}, latestSnapshot); err != nil {
			return err
		}
		r.fillSnapshotStatus(data, latestSnapshot)

		if err := r.Status().Update(ctx, latestSnapshot); err != nil {
			return err
		}
		log.Info("Persisted resolved data to status", "containerCount", len(data.containerSnapshots))
		return nil
	})
}

func (r *SandboxSnapshotReconciler) fillSnapshotStatus(data *resolvedSnapshotData, snapshot *sandboxv1alpha1.SandboxSnapshot) {
	// Update status fields only - spec is filled by Server, Controller never modifies it
	if data.sourcePodName != "" {
		snapshot.Status.SourcePodName = data.sourcePodName
	}
	if data.sourceNodeName != "" {
		snapshot.Status.SourceNodeName = data.sourceNodeName
	}
	if data.resumeTemplate != nil {
		snapshot.Status.ResumeTemplate = data.resumeTemplate
	}
	if len(data.containerSnapshots) > 0 {
		snapshot.Status.ContainerSnapshots = data.containerSnapshots
	}
}

// handlePending creates the commit Job after ensuring resolution of container snapshots
func (r *SandboxSnapshotReconciler) handlePending(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Ensure container snapshots are resolved before creating the commit job
	data, err := r.ensureResolved(ctx, snapshot)
	if err != nil {
		log.Error(err, "Failed to resolve container snapshots")
		if updateErr := r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, err.Error()); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Persist resolved data to status
	if err := r.persistResolvedData(ctx, snapshot, data); err != nil {
		log.Error(err, "Failed to persist resolved data")
		return ctrl.Result{}, err
	}
	r.fillSnapshotStatus(data, snapshot)

	// Build and create the commit Job
	job, err := r.buildCommitJob(snapshot)
	if err != nil {
		log.Error(err, "Failed to build commit job")
		if updateErr := r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, err.Error()); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Check if job already exists
	existingJob := &batchv1.Job{}
	err = r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, existingJob)
	if err == nil {
		// Job already exists, update phase to Committing
		log.Info("Commit job already exists", "job", job.Name)
		if updateErr := r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseCommitting, "Commit job created"); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	if !errors.IsNotFound(err) {
		log.Error(err, "Failed to check existing job")
		return ctrl.Result{}, err
	}

	// Create the job
	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "Failed to create commit job")
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "FailedCreateJob", "Failed to create commit job: %v", err)
		return ctrl.Result{}, err
	}

	log.Info("Created commit job", "job", job.Name)
	r.Recorder.Eventf(snapshot, corev1.EventTypeNormal, "CreatedJob", "Created commit job: %s", job.Name)

	// Update phase to Committing
	if err := r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseCommitting, "Commit job created"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second}, nil
}

// handleCommitting checks the commit Job status
func (r *SandboxSnapshotReconciler) handleCommitting(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	jobName := r.getJobName(snapshot)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: jobName}, job)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Commit job not found, re-creating", "job", jobName)
			return r.handlePending(ctx, snapshot)
		}
		log.Error(err, "Failed to get commit job")
		return ctrl.Result{}, err
	}

	// Check job status
	if job.Status.Succeeded > 0 {
		log.Info("Commit job succeeded", "job", jobName)
		r.Recorder.Eventf(snapshot, corev1.EventTypeNormal, "JobSucceeded", "Commit job succeeded")

		// ContainerSnapshots already in status from handlePending
		// Transition to Ready and append pause history record
		now := metav1.Now()
		pauseRecord := sandboxv1alpha1.SnapshotRecord{
			Action:    "Pause",
			Version:   snapshot.Status.PauseVersion,
			Timestamp: now,
			Message:   "Snapshot is ready",
		}
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			latestSnapshot := &sandboxv1alpha1.SandboxSnapshot{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: snapshot.Name}, latestSnapshot); err != nil {
				return err
			}
			latestSnapshot.Status.Phase = sandboxv1alpha1.SandboxSnapshotPhaseReady
			latestSnapshot.Status.Message = "Snapshot is ready"
			latestSnapshot.Status.ReadyAt = &now
			latestSnapshot.Status.ObservedGeneration = snapshot.Generation
			latestSnapshot.Status.History = appendHistoryRecord(latestSnapshot.Status.History, pauseRecord)
			return r.Status().Update(ctx, latestSnapshot)
		}); err != nil {
			log.Error(err, "Failed to update snapshot status to Ready")
			return ctrl.Result{}, err
		}

		log.Info("Snapshot is ready", "snapshot", snapshot.Name)
		r.Recorder.Eventf(snapshot, corev1.EventTypeNormal, "SnapshotReady", "Snapshot %s is ready", snapshot.Name)

		// Requeue to trigger handleReady for source BatchSandbox cleanup
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	if job.Status.Failed > 0 {
		log.Info("Commit job failed", "job", jobName)
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "JobFailed", "Commit job failed")

		// Get failure message from job conditions
		message := "Commit job failed"
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed {
				message = condition.Message
				break
			}
		}

		if err := r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, message); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Job still running, requeue
	log.Info("Commit job still running", "job", jobName)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// validatePauseSpec validates the required spec fields before starting a new pause cycle.
// This provides fail-fast behavior for configuration errors, avoiding creating a commit Job
// that would get stuck (e.g., missing secret causes Pod to stay in ContainerCreating).
//
// Returns (isSpecError, err):
func (r *SandboxSnapshotReconciler) validatePauseSpec(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (isValid bool, err error) {
	// snapshotRegistry is required
	if snapshot.Spec.SnapshotRegistry == "" {
		return false, fmt.Errorf("snapshotRegistry is required")
	}

	// sourceBatchSandboxName is required
	if snapshot.Spec.SourceBatchSandboxName == "" {
		return false, fmt.Errorf("sourceBatchSandboxName is required")
	}
	// If snapshotPushSecret is specified, validate it exists
	if snapshot.Spec.SnapshotPushSecret != "" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{
			Namespace: snapshot.Namespace,
			Name:      snapshot.Spec.SnapshotPushSecret,
		}, secret)
		if errors.IsNotFound(err) {
			return false, fmt.Errorf("snapshotPushSecret %q not found", snapshot.Spec.SnapshotPushSecret)
		}
		if err != nil {
			return true, err
		}
	}
	return true, nil
}

// handleReady handles a ready snapshot.
// It deletes the original (paused) BatchSandbox after the snapshot is Ready,
// If the BatchSandbox has already been resumed (marked with annotation
// sandbox.opensandbox.io/resumed-from-snapshot), deletion is skipped.
func (r *SandboxSnapshotReconciler) handleReady(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	bsName := snapshot.Spec.SourceBatchSandboxName
	if bsName == "" {
		log.Info("No source BatchSandbox specified, nothing to clean up")
		return ctrl.Result{}, nil
	}

	// Check if the source BatchSandbox still exists
	bs := &sandboxv1alpha1.BatchSandbox{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      bsName,
		Namespace: snapshot.Namespace,
	}, bs)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Source BatchSandbox already deleted", "batchSandbox", bsName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get source BatchSandbox")
		return ctrl.Result{}, err
	}

	// Only delete the BatchSandbox if the last history record is a Pause action.
	// If it was a Resume, the BatchSandbox was just created by the controller and
	// should not be deleted.
	if len(snapshot.Status.History) > 0 {
		lastRecord := snapshot.Status.History[len(snapshot.Status.History)-1]
		if lastRecord.Action != "Pause" {
			log.Info("Last action was not Pause, skipping BatchSandbox cleanup",
				"batchSandbox", bsName, "lastAction", lastRecord.Action)
			return ctrl.Result{}, nil
		}
	}

	// Delete the original (paused) BatchSandbox
	if err := r.Delete(ctx, bs, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		if errors.IsNotFound(err) {
			log.Info("BatchSandbox already gone", "batchSandbox", bsName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to delete source BatchSandbox", "batchSandbox", bsName)
		return ctrl.Result{}, err
	}

	log.Info("Deleted original (paused) BatchSandbox", "batchSandbox", bsName)
	r.Recorder.Eventf(snapshot, corev1.EventTypeNormal, "CleanedUpBatchSandbox",
		"Deleted paused BatchSandbox %s after snapshot Ready", bsName)

	return ctrl.Result{}, nil
}

// handleFailed handles a failed snapshot
func (r *SandboxSnapshotReconciler) handleFailed(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	// Snapshot failed, nothing to do
	return ctrl.Result{}, nil
}

// handleDeletion handles the deletion of a SandboxSnapshot
func (r *SandboxSnapshotReconciler) handleDeletion(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Clean up the commit job if it exists
	jobName := r.getJobName(snapshot)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: jobName}, job)
	if err == nil {
		// Delete the job
		if deleteErr := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); deleteErr != nil && !errors.IsNotFound(deleteErr) {
			log.Error(deleteErr, "Failed to delete commit job")
			return ctrl.Result{}, deleteErr
		}
		log.Info("Deleted commit job", "job", jobName)
	}

	// Remove finalizer
	if controllerutil.ContainsFinalizer(snapshot, SandboxSnapshotFinalizer) {
		if err := utils.UpdateFinalizer(r.Client, snapshot, utils.RemoveFinalizerOpType, SandboxSnapshotFinalizer); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		log.Info("Removed finalizer", "finalizer", SandboxSnapshotFinalizer)
	}

	return ctrl.Result{}, nil
}

// buildCommitJob builds a Job for committing container snapshots.
// It supports multi-container sandboxes by creating init containers for each
// container snapshot that needs to be committed, followed by a main verification container.
func (r *SandboxSnapshotReconciler) buildCommitJob(snapshot *sandboxv1alpha1.SandboxSnapshot) (*batchv1.Job, error) {
	jobName := r.getJobName(snapshot)

	// Use image-committer image (contains nerdctl tool)
	imageCommitterImage := r.ImageCommitterImage
	if imageCommitterImage == "" {
		imageCommitterImage = "image-committer:dev" // Default fallback
	}

	// Build volume mounts for containerd socket only (nerdctl connects directly, no CRI needed)
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "containerd-sock",
			MountPath: ContainerdSocketPath,
		},
	}

	// Build volumes for host paths
	volumes := []corev1.Volume{
		{
			Name: "containerd-sock",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: ContainerdSocketPath,
				},
			},
		},
	}

	// Add registry credentials from secret if specified
	if snapshot.Spec.SnapshotPushSecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "registry-creds",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: snapshot.Spec.SnapshotPushSecret,
					Items: []corev1.KeyToPath{
						{
							Key:  ".dockerconfigjson",
							Path: "config.json",
						},
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "registry-creds",
			MountPath: "/var/run/opensandbox/registry",
			ReadOnly:  true,
		})
	}

	// Build commit command using new multi-container format:
	// image-committer <pod_name> <namespace> <container1:uri1> [<container2:uri2> ...]
	containerSnapshots := snapshot.Status.ContainerSnapshots

	if len(containerSnapshots) == 0 {
		return nil, fmt.Errorf("no container snapshots specified in snapshot spec")
	}

	var containerSpecs []string
	for _, cs := range containerSnapshots {
		spec := fmt.Sprintf("%s:%s", cs.ContainerName, cs.ImageURI)
		containerSpecs = append(containerSpecs, spec)
	}
	fullCommand := fmt.Sprintf("/usr/local/bin/image-committer %s %s %s",
		snapshot.Status.SourcePodName,
		snapshot.Namespace,
		strings.Join(containerSpecs, " "),
	)

	// Build the job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: snapshot.Namespace,
			Labels: map[string]string{
				LabelSandboxSnapshotName: snapshot.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrToInt32(int32(DefaultTTLSecondsAfterFinished)),
			ActiveDeadlineSeconds:   ptrToInt64(int64(r.getCommitJobTimeout().Seconds())),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            CommitJobContainerName,
							Image:           imageCommitterImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{fullCommand},
							VolumeMounts:    volumeMounts,
							Env: []corev1.EnvVar{
								{
									// CONTAINERD_SOCKET is used by nerdctl to locate the containerd socket
									Name:  "CONTAINERD_SOCKET",
									Value: ContainerdSocketPath,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptrToInt64(0), // Run as root to access containerd
							},
						},
					},
					Volumes:  volumes,
					NodeName: snapshot.Status.SourceNodeName,
				},
			},
		},
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(snapshot, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return job, nil
}

// getJobName returns the job name for a snapshot
func (r *SandboxSnapshotReconciler) getJobName(snapshot *sandboxv1alpha1.SandboxSnapshot) string {
	return fmt.Sprintf("%s-commit-v%d", snapshot.Name, snapshot.Status.PauseVersion)
}

// updateSnapshotStatus updates the snapshot status
func (r *SandboxSnapshotReconciler) updateSnapshotStatus(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot, phase sandboxv1alpha1.SandboxSnapshotPhase, message string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latestSnapshot := &sandboxv1alpha1.SandboxSnapshot{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: snapshot.Name}, latestSnapshot); err != nil {
			return err
		}

		latestSnapshot.Status.Phase = phase
		latestSnapshot.Status.Message = message
		latestSnapshot.Status.ObservedGeneration = latestSnapshot.Generation

		return r.Status().Update(ctx, latestSnapshot)
	})
}

// getCommitJobTimeout returns the configured timeout or the default
func (r *SandboxSnapshotReconciler) getCommitJobTimeout() time.Duration {
	if r.CommitJobTimeout > 0 {
		return r.CommitJobTimeout
	}
	return DefaultCommitJobTimeout
}

// ptrToInt64 returns a pointer to an int64
func ptrToInt64(v int64) *int64 {
	return &v
}
func ptrToInt32(v int32) *int32 {
	return &v
}

// appendHistoryRecord appends a new record to history and trims to MaxHistoryRecords
func appendHistoryRecord(history []sandboxv1alpha1.SnapshotRecord, record sandboxv1alpha1.SnapshotRecord) []sandboxv1alpha1.SnapshotRecord {
	history = append(history, record)
	// Keep only the most recent MaxHistoryRecords
	if len(history) > MaxHistoryRecords {
		history = history[len(history)-MaxHistoryRecords:]
	}
	return history
}

// handleResume creates a new BatchSandbox from the snapshot resumeTemplate.
// It ACKs resumeVersion and appends a resume history record.
func (r *SandboxSnapshotReconciler) handleResume(ctx context.Context, snapshot *sandboxv1alpha1.SandboxSnapshot) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling resume request", "snapshot", snapshot.Name)

	// Validate prerequisites - ResumeTemplate is in status, filled by Controller.
	// These fields must exist when phase==Ready; a missing field indicates data corruption.
	// Set status to Failed so operators have an explicit signal and the stuck request is surfaced.
	if snapshot.Status.ResumeTemplate == nil || snapshot.Status.ResumeTemplate.Raw == nil {
		msg := "Cannot resume: resumeTemplate is missing from snapshot status"
		log.Error(fmt.Errorf("resumeTemplate is empty"), msg)
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "ResumeFailed", msg)
		return ctrl.Result{}, r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, msg)
	}

	if len(snapshot.Status.ContainerSnapshots) == 0 {
		msg := "Cannot resume: containerSnapshots is empty in snapshot status"
		log.Error(fmt.Errorf("no containerSnapshots in status"), msg)
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "ResumeFailed", msg)
		return ctrl.Result{}, r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, msg)
	}

	// Parse resumeTemplate from status
	var resumeTemplate map[string]interface{}
	if err := json.Unmarshal(snapshot.Status.ResumeTemplate.Raw, &resumeTemplate); err != nil {
		msg := fmt.Sprintf("Cannot resume: failed to parse resumeTemplate: %v", err)
		log.Error(err, "Failed to parse resumeTemplate")
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "ResumeFailed", msg)
		return ctrl.Result{}, r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, msg)
	}

	template, ok := resumeTemplate["template"].(map[string]interface{})
	if !ok {
		msg := "Cannot resume: resumeTemplate is missing 'template' field"
		log.Error(fmt.Errorf("template not found in resumeTemplate"), "Invalid resumeTemplate format")
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "ResumeFailed", msg)
		return ctrl.Result{}, r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, msg)
	}

	// Replace container images from status.ContainerSnapshots
	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		msg := "Cannot resume: resumeTemplate.template is missing 'spec' field"
		log.Error(fmt.Errorf("spec not found in template"), "Invalid template format")
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "ResumeFailed", msg)
		return ctrl.Result{}, r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, msg)
	}
	containers, ok := podSpec["containers"].([]interface{})
	if !ok {
		msg := "Cannot resume: resumeTemplate.template.spec is missing 'containers' field"
		log.Error(fmt.Errorf("containers not found in template spec"), "Invalid template format")
		r.Recorder.Eventf(snapshot, corev1.EventTypeWarning, "ResumeFailed", msg)
		return ctrl.Result{}, r.updateSnapshotStatus(ctx, snapshot, sandboxv1alpha1.SandboxSnapshotPhaseFailed, msg)
	}
	for _, cs := range snapshot.Status.ContainerSnapshots {
		for i, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if container["name"] == cs.ContainerName {
				container["image"] = cs.ImageURI
				containers[i] = container
				break
			}
		}
	}

	// Add imagePullSecrets from spec
	if snapshot.Spec.ResumeImagePullSecret != "" {
		podSpec["imagePullSecrets"] = []interface{}{
			map[string]interface{}{"name": snapshot.Spec.ResumeImagePullSecret},
		}
	}

	// Build BatchSandbox manifest
	// Note: pausePolicy is NOT copied - BatchSandbox no longer has this field
	// Pause config comes from server config, not from CRD
	bsSpec := map[string]interface{}{
		"replicas": 1,
		"template": template,
	}

	// Add expireTime from resumeTemplate if present
	if expireTime, ok := resumeTemplate["expireTime"]; ok && expireTime != nil {
		bsSpec["expireTime"] = expireTime
	}

	// Merge user labels from resumeTemplate.metadata.labels (saved at pause time) into
	// the new BatchSandbox so metadata is stable across running→paused→resumed transitions.
	bsLabels := map[string]interface{}{}
	if metadataRaw, ok := resumeTemplate["metadata"]; ok {
		if metadataMap, ok := metadataRaw.(map[string]interface{}); ok {
			if labelsRaw, ok := metadataMap["labels"]; ok {
				if labelsMap, ok := labelsRaw.(map[string]interface{}); ok {
					for k, v := range labelsMap {
						bsLabels[k] = v
					}
				}
			}
		}
	}
	// System labels override any user labels with the same key.
	bsLabels["sandbox.opensandbox.io/sandbox-id"] = snapshot.Spec.SandboxID
	bsLabels["sandbox.opensandbox.io/resumed-from-snapshot"] = "true"
	bsLabels[LabelBatchSandboxNameKey] = snapshot.Spec.SandboxID

	batchsandboxManifest := map[string]interface{}{
		"apiVersion": fmt.Sprintf("%s/%s", sandboxv1alpha1.GroupVersion.Group, sandboxv1alpha1.GroupVersion.Version),
		"kind":       "BatchSandbox",
		"metadata": map[string]interface{}{
			"name":      snapshot.Spec.SandboxID,
			"namespace": snapshot.Namespace,
			"labels":    bsLabels,
			"annotations": map[string]interface{}{
				"sandbox.opensandbox.io/resumed-from-snapshot": "true",
			},
		},
		"spec": bsSpec,
	}

	// Create BatchSandbox using unstructured
	bsJSON, err := json.Marshal(batchsandboxManifest)
	if err != nil {
		log.Error(err, "Failed to marshal BatchSandbox manifest")
		return ctrl.Result{}, err
	}

	unstructuredBS := &unstructured.Unstructured{}
	if err := unstructuredBS.UnmarshalJSON(bsJSON); err != nil {
		log.Error(err, "Failed to decode BatchSandbox manifest")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, unstructuredBS); err != nil {
		if errors.IsAlreadyExists(err) {
			log.Info("BatchSandbox already exists, resume may have been processed", "name", snapshot.Spec.SandboxID)
		} else {
			log.Error(err, "Failed to create BatchSandbox")
			return ctrl.Result{}, err
		}
	}

	log.Info("Created BatchSandbox from snapshot", "name", snapshot.Spec.SandboxID)
	r.Recorder.Eventf(snapshot, corev1.EventTypeNormal, "ResumedBatchSandbox",
		"Created BatchSandbox %s from snapshot", snapshot.Spec.SandboxID)

	// ACK resume: increment internal resume counter, set observedGeneration and LastResumeAt
	newResumeVersion := snapshot.Status.ResumeVersion + 1
	now := metav1.Now()
	resumeRecord := sandboxv1alpha1.SnapshotRecord{
		Action:    "Resume",
		Version:   newResumeVersion,
		Timestamp: now,
		Message:   fmt.Sprintf("Resumed to BatchSandbox %s", snapshot.Spec.SandboxID),
	}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latestSnapshot := &sandboxv1alpha1.SandboxSnapshot{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: snapshot.Name}, latestSnapshot); err != nil {
			return err
		}
		latestSnapshot.Status.ResumeVersion = newResumeVersion
		latestSnapshot.Status.ObservedGeneration = snapshot.Generation
		latestSnapshot.Status.LastResumeAt = &now
		latestSnapshot.Status.History = appendHistoryRecord(latestSnapshot.Status.History, resumeRecord)
		return r.Status().Update(ctx, latestSnapshot)
	}); err != nil {
		log.Error(err, "Failed to ACK resume")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// convertPodTemplateSpecToMap converts a PodTemplateSpec to a map[string]interface{}
func convertPodTemplateSpecToMap(template *corev1.PodTemplateSpec) map[string]interface{} {
	if template == nil {
		return nil
	}

	result := make(map[string]interface{})

	// Convert ObjectMeta
	if !template.ObjectMeta.CreationTimestamp.IsZero() || len(template.ObjectMeta.Labels) > 0 || len(template.ObjectMeta.Annotations) > 0 {
		meta := make(map[string]interface{})
		if len(template.ObjectMeta.Labels) > 0 {
			meta["labels"] = template.ObjectMeta.Labels
		}
		if len(template.ObjectMeta.Annotations) > 0 {
			meta["annotations"] = template.ObjectMeta.Annotations
		}
		result["metadata"] = meta
	}

	// Convert PodSpec
	podSpecBytes, _ := json.Marshal(template.Spec)
	var podSpecMap map[string]interface{}
	_ = json.Unmarshal(podSpecBytes, &podSpecMap)
	if podSpecMap != nil {
		result["spec"] = podSpecMap
	}

	return result
}

// convertToRawExtension converts a struct to RawExtension
func convertToRawExtension(data interface{}) (runtime.RawExtension, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return runtime.RawExtension{}, err
	}

	return runtime.RawExtension{
		Raw: jsonBytes,
	}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *SandboxSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sandboxv1alpha1.SandboxSnapshot{}).
		Owns(&batchv1.Job{}).
		Named("sandboxsnapshot").
		Complete(r)
}
