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
	gerrors "errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/controller/strategy"
	taskscheduler "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/scheduler"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils"
	controllerutils "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/controller"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/expectations"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/fieldindex"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/requeueduration"
	taskexecutor "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/task-executor"
)

var (
	BatchSandboxScaleExpectations = expectations.NewScaleExpectations()
	DurationStore                 = requeueduration.DurationStore{}
)

// ensureFinalizer ensures the finalizer is added to the object.
// Returns true if the finalizer was added (false if already exists).
func (r *BatchSandboxReconciler) ensureFinalizer(ctx context.Context, obj client.Object, finalizer string) (bool, error) {
	log := logf.FromContext(ctx)
	if controllerutil.ContainsFinalizer(obj, finalizer) {
		return false, nil
	}
	if err := utils.UpdateFinalizer(r.Client, obj, utils.AddFinalizerOpType, finalizer); err != nil {
		log.Error(err, "failed to add finalizer", "finalizer", finalizer)
		return false, err
	}
	log.Info("added finalizer", "finalizer", finalizer)
	return true, nil
}

// BatchSandboxReconciler reconciles a BatchSandbox object
type BatchSandboxReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	taskSchedulers sync.Map
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=batchsandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=batchsandboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=batchsandboxes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BatchSandbox object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *BatchSandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var aggErrors []error
	defer func() {
		_ = DurationStore.Pop(req.String())
	}()
	batchSbx := &sandboxv1alpha1.BatchSandbox{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: req.Namespace,
		Name:      req.Name,
	}, batchSbx); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	// handle expire
	if expireAt := batchSbx.Spec.ExpireTime; expireAt != nil {
		now := time.Now()
		if expireAt.Time.Before(now) {
			if batchSbx.DeletionTimestamp == nil {
				log.Info("batch sandbox expired, delete", "expireAt", expireAt)
				if err := r.Delete(ctx, batchSbx); err != nil {
					if errors.IsNotFound(err) {
						return ctrl.Result{}, nil
					}
					return ctrl.Result{}, err
				}
			}
		} else {
			DurationStore.Push(types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String(), expireAt.Time.Sub(now))
		}
	}

	// task schedule
	taskStrategy := strategy.NewTaskSchedulingStrategy(batchSbx)

	// pool strategy
	poolStrategy := strategy.NewPoolStrategy(batchSbx)

	// handle finalizers
	if batchSbx.DeletionTimestamp == nil {
		// Add task cleanup finalizer for BatchSandbox with task scheduling
		if taskStrategy.NeedTaskScheduling() {
			if added, err := r.ensureFinalizer(ctx, batchSbx, FinalizerTaskCleanup); added || err != nil {
				return ctrl.Result{}, err
			}
		}
		// Add pod disposal finalizer for pooled BatchSandbox
		if poolStrategy.IsPooledMode() {
			if added, err := r.ensureFinalizer(ctx, batchSbx, FinalizerPodDisposal); added || err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// Handle pod disposal before final cleanup
		// This must be done before returning, regardless of task scheduling needs
		if poolStrategy.IsPooledMode() && controllerutil.ContainsFinalizer(batchSbx, FinalizerPodDisposal) {
			result, err := r.handlePodDisposal(ctx, batchSbx)
			if err != nil {
				log.Error(err, "failed to handle pod disposal")
				return ctrl.Result{}, err
			}
			if result.RequeueAfter > 0 {
				return result, nil
			}
			// Pod disposal completed, remove finalizer
			if err := utils.UpdateFinalizer(r.Client, batchSbx, utils.RemoveFinalizerOpType, FinalizerPodDisposal); err != nil {
				log.Error(err, "failed to remove finalizer", "finalizer", FinalizerPodDisposal)
				return ctrl.Result{}, err
			}
			log.Info("pod disposal completed, removed finalizer", "finalizer", FinalizerPodDisposal)
		}
		if !taskStrategy.NeedTaskScheduling() {
			return ctrl.Result{}, nil
		}
	}

	pods, err := r.listPods(ctx, poolStrategy, batchSbx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods %w", err)
	}
	podIndex, err := calPodIndex(poolStrategy, batchSbx, pods)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cal pod index %w", err)
	}
	slices.SortStableFunc(pods, utils.MultiPodSorter([]func(a, b *corev1.Pod) int{
		utils.WithPodIndexSorter(podIndex),
		utils.PodNameSorter,
	}).Sort)
	// Normal Mode need scale Pods
	if !poolStrategy.IsPooledMode() {
		err := r.scaleBatchSandbox(ctx, batchSbx, batchSbx.Spec.Template, pods)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to scale batch sandbox %w", err)
		}
	}

	// TODO merge task status update
	newStatus := batchSbx.Status.DeepCopy()
	newStatus.ObservedGeneration = batchSbx.Generation
	newStatus.Replicas = 0
	newStatus.Allocated = 0
	newStatus.Ready = 0
	ipList := make([]string, len(pods))
	for i, pod := range pods {
		newStatus.Replicas++
		if utils.IsAssigned(pod) {
			newStatus.Allocated++
			ipList[i] = pod.Status.PodIP
		}
		if pod.Status.Phase == corev1.PodRunning && utils.IsPodReady(pod) {
			newStatus.Ready++
		}
	}
	raw, _ := json.Marshal(ipList)
	if batchSbx.Annotations[AnnotationSandboxEndpoints] != string(raw) {
		patchData, _ := json.Marshal(map[string]any{
			"metadata": map[string]any{
				"annotations": map[string]string{
					AnnotationSandboxEndpoints: string(raw),
				},
			},
		})
		obj := &sandboxv1alpha1.BatchSandbox{ObjectMeta: metav1.ObjectMeta{Namespace: batchSbx.Namespace, Name: batchSbx.Name}}
		if err := r.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			log.Error(err, "failed to patch annotation", "annotation", AnnotationSandboxEndpoints, "body", string(patchData))
			aggErrors = append(aggErrors, err)
		}
	}
	if !reflect.DeepEqual(newStatus, batchSbx.Status) {
		log.Info("To update BatchSandbox status", "replicas", newStatus.Replicas, "allocated", newStatus.Allocated, "ready", newStatus.Ready)
		if err := r.updateStatus(batchSbx, newStatus); err != nil {
			aggErrors = append(aggErrors, err)
		}
	}

	if taskStrategy.NeedTaskScheduling() {
		// Because tasks are in-memory and there is no event mechanism, periodic reconciliation is required.
		DurationStore.Push(types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String(), 3*time.Second)
		sch, err := r.getTaskScheduler(ctx, batchSbx, pods)
		if err != nil {
			return ctrl.Result{}, err
		}
		if batchSbx.DeletionTimestamp != nil {
			stoppingTasks := sch.StopTask()
			if len(stoppingTasks) > 0 {
				log.Info("stopping tasks", "count", len(stoppingTasks))
			}
		}
		now := time.Now()
		if err = r.scheduleTasks(ctx, sch, batchSbx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to schedule tasks, err %w", err)
		} else {
			log.Info("schedule tasks completed", "costMs", time.Since(now).Milliseconds())
		}
		// check task cleanup is finished
		if batchSbx.DeletionTimestamp != nil {
			unfinishedTasks := r.getTasksCleanupUnfinished(batchSbx, sch)
			if len(unfinishedTasks) > 0 {
				log.Info("tasks cleanup is unfinished", "unfinishedCount", len(unfinishedTasks))
			} else {
				var err error
				if controllerutil.ContainsFinalizer(batchSbx, FinalizerTaskCleanup) {
					err = utils.UpdateFinalizer(r.Client, batchSbx, utils.RemoveFinalizerOpType, FinalizerTaskCleanup)
					if err != nil {
						if errors.IsNotFound(err) {
							err = nil
						} else {
							log.Error(err, "failed to remove finalizer", "finalizer", FinalizerTaskCleanup)
						}
					}
				}
				if err == nil {
					r.deleteTaskScheduler(ctx, batchSbx)
					log.Info("task cleanup is finished, removed finalizer", "finalizer", FinalizerTaskCleanup)
				}
				return ctrl.Result{}, err
			}
		}
	}

	return reconcile.Result{RequeueAfter: DurationStore.Pop(req.String())}, gerrors.Join(aggErrors...)
}

func calPodIndex(poolStrategy strategy.PoolStrategy, batchSbx *sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (map[string]int, error) {
	podIndex := map[string]int{}
	if poolStrategy.IsPooledMode() {
		// cal index from pool alloc result while using pooling
		alloc, err := parseSandboxAllocation(batchSbx)
		if err != nil {
			return nil, err
		}
		for i := range alloc.Pods {
			podIndex[alloc.Pods[i]] = i
		}
	} else {
		for i := range pods {
			po := pods[i]
			idx, err := parseIndex(po)
			if err != nil {
				return nil, fmt.Errorf("batchsandbox: failed to parse %s/%s index %w", po.Namespace, po.Name, err)
			}
			podIndex[po.Name] = idx
		}
	}
	return podIndex, nil
}

func (r *BatchSandboxReconciler) listPods(ctx context.Context, poolStrategy strategy.PoolStrategy, batchSbx *sandboxv1alpha1.BatchSandbox) ([]*corev1.Pod, error) {
	var ret []*corev1.Pod
	if poolStrategy.IsPooledMode() {
		var (
			allocSet    = make(sets.Set[string])
			releasedSet = make(sets.Set[string])
		)
		alloc, err := parseSandboxAllocation(batchSbx)
		if err != nil {
			return nil, err
		}
		allocSet.Insert(alloc.Pods...)

		released, err := parseSandboxReleased(batchSbx)
		if err != nil {
			return nil, err
		}
		releasedSet.Insert(released.Pods...)

		activePods := allocSet.Difference(releasedSet)
		for name := range activePods {
			pod := &corev1.Pod{}
			// TODO maybe performance is problem
			if err := r.Client.Get(ctx, types.NamespacedName{Namespace: batchSbx.Namespace, Name: name}, pod); err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return nil, err
			}
			ret = append(ret, pod)
		}
	} else {
		podList := &corev1.PodList{}
		if err := r.Client.List(ctx, podList, &client.ListOptions{
			Namespace:     batchSbx.Namespace,
			FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(batchSbx.UID)}),
		}); err != nil {
			return nil, err
		}
		for i := range podList.Items {
			ret = append(ret, &podList.Items[i])
		}
	}
	return ret, nil
}

func (r *BatchSandboxReconciler) getTaskScheduler(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (taskscheduler.TaskScheduler, error) {
	log := logf.FromContext(ctx)
	var tSch taskscheduler.TaskScheduler
	key := types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String()
	val, ok := r.taskSchedulers.Load(key)
	// The reconciler guarantees that it will not concurrently reconcile the same BatchSandbox.
	if !ok {
		policy := sandboxv1alpha1.TaskResourcePolicyRetain
		if batchSbx.Spec.TaskResourcePolicyWhenCompleted != nil {
			policy = *batchSbx.Spec.TaskResourcePolicyWhenCompleted
		}
		taskStrategy := strategy.NewTaskSchedulingStrategy(batchSbx)
		taskSpecs, err := taskStrategy.GenerateTaskSpecs()
		if err != nil {
			return nil, err
		}
		sc, err := taskscheduler.NewTaskScheduler(key, taskSpecs, pods, policy, log)
		if err != nil {
			return nil, fmt.Errorf("new task scheduler err %w", err)
		}
		log.Info("successfully created task scheduler")
		tSch = sc
		r.taskSchedulers.Store(key, sc)
	} else {
		tSch, ok = (val.(taskscheduler.TaskScheduler))
		if !ok {
			return nil, gerrors.New("invalid scheduler type stored")
		}
		// Update the pods list for this scheduler
		tSch.UpdatePods(pods)
	}
	return tSch, nil
}

func (r *BatchSandboxReconciler) deleteTaskScheduler(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox) {
	log := logf.FromContext(ctx)
	log.Info("delete task scheduler")
	key := types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String()
	r.taskSchedulers.Delete(key)
}

func (r *BatchSandboxReconciler) scheduleTasks(ctx context.Context, tSch taskscheduler.TaskScheduler, batchSbx *sandboxv1alpha1.BatchSandbox) error {
	log := logf.FromContext(ctx)
	if err := tSch.Schedule(); err != nil {
		return err
	}
	tasks := tSch.ListTask()
	toReleasedPods := []string{}
	var (
		running, failed, succeed, unknown int32
		pending                           int32
	)
	for i := range len(tasks) {
		task := tasks[i]
		if task.GetPodName() == "" {
			pending++
		} else {
			state := task.GetState()
			if task.IsResourceReleased() {
				toReleasedPods = append(toReleasedPods, task.GetPodName())
			}
			switch state {
			case taskscheduler.RunningTaskState:
				running++
			case taskscheduler.SucceedTaskState:
				succeed++
			case taskscheduler.FailedTaskState:
				failed++
			case taskscheduler.UnknownTaskState:
				unknown++
			}
		}
	}
	if len(toReleasedPods) > 0 {
		log.Info("try to release Pods", "count", len(toReleasedPods))
		if err := r.releasePods(ctx, batchSbx, toReleasedPods); err != nil {
			return err
		}
		log.Info("successfully released Pods", "count", len(toReleasedPods))
	}
	oldStatus := batchSbx.Status
	newStatus := oldStatus.DeepCopy()
	newStatus.ObservedGeneration = batchSbx.Generation
	newStatus.TaskRunning = running
	newStatus.TaskFailed = failed
	newStatus.TaskSucceed = succeed
	newStatus.TaskUnknown = unknown
	newStatus.TaskPending = pending
	if !reflect.DeepEqual(newStatus, oldStatus) {
		log.Info("To update BatchSandbox status", "replicas", newStatus.Replicas, "task_running", newStatus.TaskRunning, "task_succeed", newStatus.TaskSucceed, "task_failed", newStatus.TaskFailed, "task_unknown", newStatus.TaskUnknown, "task_pending", newStatus.TaskPending)
		if err := r.updateStatus(batchSbx, newStatus); err != nil {
			return err
		}
	}
	return nil
}

func (r *BatchSandboxReconciler) getTasksCleanupUnfinished(batchSbx *sandboxv1alpha1.BatchSandbox, tSch taskscheduler.TaskScheduler) []taskscheduler.Task {
	var notReleased []taskscheduler.Task
	for _, task := range tSch.ListTask() {
		if !task.IsResourceReleased() {
			notReleased = append(notReleased, task)
		}
	}
	return notReleased
}

func (r *BatchSandboxReconciler) releasePods(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox, toReleasePods []string) error {
	releasedSet := make(sets.Set[string])
	released, err := parseSandboxReleased(batchSbx)
	if err != nil {
		return err
	}
	releasedSet.Insert(released.Pods...)
	releasedSet.Insert(toReleasePods...)
	newRelease := AllocationRelease{
		Pods: sets.List(releasedSet),
	}
	raw, err := json.Marshal(newRelease)
	if err != nil {
		return fmt.Errorf("Failed to marshal released pod names: %v", err)
	}
	body := utils.DumpJSON(struct {
		MetaData metav1.ObjectMeta `json:"metadata"`
	}{
		MetaData: metav1.ObjectMeta{
			Annotations: map[string]string{
				AnnoAllocReleaseKey: string(raw),
			},
		},
	})
	b := &sandboxv1alpha1.BatchSandbox{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: batchSbx.Namespace,
			Name:      batchSbx.Name,
		},
	}
	return r.Client.Patch(ctx, b, client.RawPatch(types.MergePatchType, []byte(body)))
}

// Normal Mode
func (r *BatchSandboxReconciler) scaleBatchSandbox(ctx context.Context, batchSandbox *sandboxv1alpha1.BatchSandbox, podTemplateSpec *corev1.PodTemplateSpec, pods []*corev1.Pod) error {
	log := logf.FromContext(ctx)
	indexedPodMap := map[int]*corev1.Pod{}
	for i := range pods {
		pod := pods[i]
		BatchSandboxScaleExpectations.ObserveScale(controllerutils.GetControllerKey(batchSandbox), expectations.Create, pod.Name)
		pods = append(pods, pod)
		idx, err := parseIndex(pod)
		if err != nil {
			return fmt.Errorf("failed to parse idx Pod %s, err %w", pod.Name, err)
		}
		indexedPodMap[idx] = pod
	}
	if satisfied, unsatisfiedDuration, dirtyPods := BatchSandboxScaleExpectations.SatisfiedExpectations(controllerutils.GetControllerKey(batchSandbox)); !satisfied {
		log.Info("scale expectation is not satisfied", "unsatisfiedDuration", unsatisfiedDuration, "dirtyPods", dirtyPods)
		DurationStore.Push(types.NamespacedName{Namespace: batchSandbox.Namespace, Name: batchSandbox.Name}.String(), expectations.ExpectationTimeout-unsatisfiedDuration)
		return nil
	}
	// TODO consider supply Pods if Pods is deleted unexpectedly
	var needCreateIndex []int
	// TODO var needDeleteIndex []int
	for i := 0; i < int(*batchSandbox.Spec.Replicas); i++ {
		_, ok := indexedPodMap[i]
		if !ok {
			needCreateIndex = append(needCreateIndex, i)
		}
	}
	// scale
	if len(needCreateIndex) > 0 {
		log.Info("try to create Pods", "count", len(needCreateIndex), "indexes", needCreateIndex)
	}
	for _, idx := range needCreateIndex {
		pod, err := utils.GetPodFromTemplate(podTemplateSpec, batchSandbox, metav1.NewControllerRef(batchSandbox, sandboxv1alpha1.SchemeBuilder.GroupVersion.WithKind("BatchSandbox")))
		if err != nil {
			return err
		}
		// Apply shard patch if available for this index
		if len(batchSandbox.Spec.ShardPatches) > 0 && idx < len(batchSandbox.Spec.ShardPatches) {
			podBytes, err := json.Marshal(pod)
			if err != nil {
				return fmt.Errorf("failed to marshal pod: %w", err)
			}
			patch := batchSandbox.Spec.ShardPatches[idx]
			modifiedPodBytes, err := strategicpatch.StrategicMergePatch(podBytes, patch.Raw, &corev1.Pod{})
			if err != nil {
				return fmt.Errorf("failed to apply shard patch for index %d: %w", idx, err)
			}
			if err := json.Unmarshal(modifiedPodBytes, pod); err != nil {
				return fmt.Errorf("failed to unmarshal patched pod for index %d: %w", idx, err)
			}
		}
		if err := ctrl.SetControllerReference(pod, batchSandbox, r.Scheme); err != nil {
			return err
		}
		pod.Labels[LabelBatchSandboxPodIndexKey] = strconv.Itoa(idx)
		pod.Namespace = batchSandbox.Namespace
		pod.Name = fmt.Sprintf("%s-%d", batchSandbox.Name, idx)
		BatchSandboxScaleExpectations.ExpectScale(controllerutils.GetControllerKey(batchSandbox), expectations.Create, pod.Name)
		if err := r.Create(ctx, pod); err != nil {
			BatchSandboxScaleExpectations.ObserveScale(controllerutils.GetControllerKey(batchSandbox), expectations.Create, pod.Name)
			r.Recorder.Eventf(batchSandbox, corev1.EventTypeWarning, "FailedCreate", "failed to create pod: %v, pod: %v", err, utils.DumpJSON(pod))
			return err
		}
		r.Recorder.Eventf(batchSandbox, corev1.EventTypeNormal, "SuccessfulCreate", "succeed to create pod %s", pod.Name)
	}
	return nil
}

func parseIndex(pod *corev1.Pod) (int, error) {
	if v := pod.Labels[LabelBatchSandboxPodIndexKey]; v != "" {
		return strconv.Atoi(v)
	}
	idx := strings.LastIndex(pod.Name, "-")
	if idx == -1 {
		return -1, gerrors.New("batchsandbox: Invalid pod Name")
	}
	return strconv.Atoi(pod.Name[idx+1:])
}

func (r *BatchSandboxReconciler) updateStatus(batchSandbox *sandboxv1alpha1.BatchSandbox, newStatus *sandboxv1alpha1.BatchSandboxStatus) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone := &sandboxv1alpha1.BatchSandbox{}
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: batchSandbox.Namespace, Name: batchSandbox.Name}, clone); err != nil {
			return err
		}
		clone.Status = *newStatus
		return r.Status().Update(context.TODO(), clone)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *BatchSandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sandboxv1alpha1.BatchSandbox{}).
		Named("batchsandbox").
		Owns(&corev1.Pod{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 32}).
		Complete(r)
}

// handlePodDisposal handles pod disposal when a pooled BatchSandbox is being deleted.
// It returns ctrl.Result with RequeueAfter > 0 if the operation is in progress.
func (r *BatchSandboxReconciler) handlePodDisposal(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	pool := &sandboxv1alpha1.Pool{}
	if err := r.Get(ctx, types.NamespacedName{Name: batchSbx.Spec.PoolRef, Namespace: batchSbx.Namespace}, pool); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Pool not found, skipping pod disposal", "pool", batchSbx.Spec.PoolRef)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	alloc, err := parseSandboxAllocation(batchSbx)
	if err != nil {
		return ctrl.Result{}, err
	}

	policy := sandboxv1alpha1.PodRecyclePolicyDelete
	if pool.Spec.PodRecyclePolicy != "" {
		policy = pool.Spec.PodRecyclePolicy
	}

	anyPodResetting := false

	for _, podName := range alloc.Pods {
		pod := &corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: batchSbx.Namespace}, pod); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			log.Error(err, "Failed to get pod", "pod", podName)
			continue
		}

		// supportsReuse is determined by pool controller at pod creation time:
		// Pool's PodRecyclePolicy is Reuse and PoolReconciler has TaskExecutorImage configured
		supportsReuse := pod.Labels[LabelPodReuseEnabled] == "true"

		if policy == sandboxv1alpha1.PodRecyclePolicyReuse && supportsReuse {
			result, err := r.handleReusePolicy(ctx, batchSbx, pool, pod)
			if err != nil {
				log.Error(err, "Failed to handle reuse policy, falling back to delete", "pod", podName)
				if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete pod after reuse policy failure", "pod", podName)
				}
				continue
			}
			if result.RequeueAfter > 0 {
				anyPodResetting = true
			}
		} else {
			if policy == sandboxv1alpha1.PodRecyclePolicyReuse {
				log.Info("Pod does not support reuse, falling back to delete",
					"pod", podName,
					"pool", pool.Name,
					"reason", "task-executor not injected (controller may not have task-executor-image configured)")
			}
			// Delete the pod directly (don't release back to pool)
			if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete pod", "pod", podName)
			} else {
				log.Info("Pod deleted for Delete policy", "pod", podName, "pool", pool.Name)
			}
		}
	}

	if anyPodResetting {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// handleReusePolicy handles the Reuse policy for a single pod.
func (r *BatchSandboxReconciler) handleReusePolicy(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox, pool *sandboxv1alpha1.Pool, pod *corev1.Pod) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	recycleState := pod.Labels[LabelPodRecycleState]

	switch recycleState {
	case PodRecycleStateResetting:
		// Must poll the Reset API to detect completion/failure
		log.Info("Pod is being reset, polling status", "pod", pod.Name)
		return r.pollResetStatus(ctx, batchSbx, pool, pod)

	case PodRecycleStateResetSucceeded:
		// Clear the recycle state label. If cleanup fails, requeue to retry.
		log.Info("Pod reset succeeded, clearing recycle state label", "pod", pod.Name)
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		delete(pod.Labels, LabelPodRecycleState)
		if err := r.Update(ctx, pod); err != nil {
			log.Error(err, "Failed to clear recycle state label", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, nil

	case PodRecycleStateResetFailed:
		log.Info("Pod reset failed, deleting", "pod", pod.Name)
		if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	default:
		log.Info("Starting pod reset", "pod", pod.Name)
		if err := r.startPodReset(ctx, batchSbx, pool, pod); err != nil {
			log.Error(err, "Failed to start pod reset", "pod", pod.Name)
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			pod.Labels[LabelPodRecycleState] = PodRecycleStateResetFailed
			if err := r.Update(ctx, pod); err != nil {
				log.Error(err, "Failed to mark pod reset failed", "pod", pod.Name)
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

// callResetAPI calls the task-executor Reset API and returns the response.
func (r *BatchSandboxReconciler) callResetAPI(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox, pool *sandboxv1alpha1.Pool, pod *corev1.Pod) (*taskexecutor.ResetResponse, error) {
	log := logf.FromContext(ctx)

	timeoutSeconds := int64(60)
	cleanDirectories := []string{"/tmp"}
	mainContainerName := ""

	if pool.Spec.ResetSpec != nil {
		if pool.Spec.ResetSpec.TimeoutSeconds > 0 {
			timeoutSeconds = pool.Spec.ResetSpec.TimeoutSeconds
		}
		if len(pool.Spec.ResetSpec.CleanDirectories) > 0 {
			cleanDirectories = pool.Spec.ResetSpec.CleanDirectories
		}
		mainContainerName = pool.Spec.ResetSpec.MainContainerName
	}

	if pod.Status.PodIP == "" {
		return nil, fmt.Errorf("pod has no IP assigned")
	}

	client := taskexecutor.NewClientWithTimeout(
		fmt.Sprintf("http://%s:%s", pod.Status.PodIP, taskexecutor.GetPort()),
		time.Duration(timeoutSeconds+30)*time.Second,
	)

	resetReq := &taskexecutor.ResetRequest{
		Version:           string(batchSbx.UID),
		TimeoutSeconds:    timeoutSeconds,
		CleanDirectories:  cleanDirectories,
		MainContainerName: mainContainerName,
	}

	log.V(1).Info("Calling task-executor reset API", "pod", pod.Name, "podIP", pod.Status.PodIP, "version", resetReq.Version)

	return client.Reset(ctx, resetReq)
}

// handleResetResponse updates the pod state based on the Reset API response.
func (r *BatchSandboxReconciler) handleResetResponse(ctx context.Context, pod *corev1.Pod, resp *taskexecutor.ResetResponse) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	switch resp.Status {
	case taskexecutor.ResetStatusSuccess:
		log.Info("Pod reset completed successfully", "pod", pod.Name, "details", resp.Details)
		pod.Labels[LabelPodRecycleState] = PodRecycleStateResetSucceeded
		if err := r.Update(ctx, pod); err != nil {
			log.Error(err, "Failed to update pod reset state", "pod", pod.Name)
		}
		return ctrl.Result{}, nil

	case taskexecutor.ResetStatusInProgress:
		log.V(1).Info("Pod reset in progress", "pod", pod.Name)
		pod.Labels[LabelPodRecycleState] = PodRecycleStateResetting
		if err := r.Update(ctx, pod); err != nil {
			log.Error(err, "Failed to update pod reset state", "pod", pod.Name)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	default:
		log.Error(nil, "Pod reset failed", "pod", pod.Name, "status", resp.Status, "message", resp.Message)
		pod.Labels[LabelPodRecycleState] = PodRecycleStateResetFailed
		if err := r.Update(ctx, pod); err != nil {
			log.Error(err, "Failed to update pod reset state", "pod", pod.Name)
		}
		return ctrl.Result{}, nil
	}
}

// pollResetStatus polls the Reset API to detect completion or failure.
func (r *BatchSandboxReconciler) pollResetStatus(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox, pool *sandboxv1alpha1.Pool, pod *corev1.Pod) (ctrl.Result, error) {
	resp, err := r.callResetAPI(ctx, batchSbx, pool, pod)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to poll reset status", "pod", pod.Name)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return r.handleResetResponse(ctx, pod, resp)
}

// startPodReset starts the reset process for a pod.
func (r *BatchSandboxReconciler) startPodReset(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox, pool *sandboxv1alpha1.Pool, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[LabelPodRecycleState] = PodRecycleStateResetting
	if err := r.Update(ctx, pod); err != nil {
		return err
	}

	resp, err := r.callResetAPI(ctx, batchSbx, pool, pod)
	if err != nil {
		log.Error(err, "Failed to call task-executor reset API", "pod", pod.Name)
		return err
	}

	_, err = r.handleResetResponse(ctx, pod, resp)
	return err
}
