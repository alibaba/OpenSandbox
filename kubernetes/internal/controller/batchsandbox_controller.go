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
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/api/v1alpha1"
	taskscheduler "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/scheduler"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils"
	controllerutils "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/controller"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/expectations"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/fieldindex"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/requeueduration"
)

var (
	BatchSandboxScaleExpectations = expectations.NewScaleExpectations()
	DurationStore                 = requeueduration.DurationStore{}
)

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
				klog.Infof("batch sandbox %s expired, expire at %v, delete", klog.KObj(batchSbx), expireAt)
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

	// handle finalizers
	if batchSbx.DeletionTimestamp == nil {
		if batchSbx.Spec.TaskTemplate != nil {
			if !controllerutil.ContainsFinalizer(batchSbx, FinalizerTaskCleanup) {
				err := utils.UpdateFinalizer(r.Client, batchSbx, utils.AddFinalizerOpType, FinalizerTaskCleanup)
				if err != nil {
					klog.Errorf("failed to add finalizer %s %s, err %v", FinalizerTaskCleanup, klog.KObj(batchSbx), err)
				} else {
					klog.Infof("batchsandbox %s add finalizer %s", klog.KObj(batchSbx), FinalizerTaskCleanup)
				}
				return ctrl.Result{}, err
			}
		}
	} else {
		if batchSbx.Spec.TaskTemplate == nil {
			return ctrl.Result{}, nil
		}
	}

	var aggErrors []error

	pods, err := r.listPods(ctx, batchSbx)
	if err != nil {
		aggErrors = append(aggErrors, err)
	} else {
		slices.SortStableFunc[[]*corev1.Pod, *corev1.Pod](pods, utils.PodNameSorter)
		var err error
		// Normal Mode need scale Pods
		if batchSbx.Spec.Template != nil {
			err = r.scaleBatchSandbox(ctx, batchSbx, batchSbx.Spec.Template, pods)
		}
		if err != nil {
			aggErrors = append(aggErrors, err)
		}

		// TODO merge task status update
		newStatus := batchSbx.Status.DeepCopy()
		newStatus.ObservedGeneration = batchSbx.Generation
		newStatus.Replicas = 0
		newStatus.Allocated = 0
		newStatus.Ready = 0
		ipList := []string{}
		for _, pod := range pods {
			newStatus.Replicas++
			if utils.IsAssigned(pod) {
				newStatus.Allocated++
				ipList = append(ipList, pod.Status.PodIP)
			}
			if pod.Status.Phase == corev1.PodRunning && utils.IsPodReady(pod) {
				newStatus.Ready++
			}
		}
		raw, _ := json.Marshal(ipList)
		if batchSbx.Annotations[AnnotationSandboxEndpoints] != string(raw) {
			patchData, _ := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						AnnotationSandboxEndpoints: string(raw),
					},
				},
			})
			obj := &sandboxv1alpha1.BatchSandbox{ObjectMeta: metav1.ObjectMeta{Namespace: batchSbx.Namespace, Name: batchSbx.Name}}
			if err := r.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patchData)); err != nil {
				klog.Errorf("failed to patch annotation %s, %s, body %s", AnnotationSandboxEndpoints, klog.KObj(batchSbx), patchData)
				aggErrors = append(aggErrors, err)
			}
		}
		if !reflect.DeepEqual(newStatus, batchSbx.Status) {
			klog.Infof("To update BatchSandbox status for %s, replicas=%d allocated=%d ready=%d", klog.KObj(batchSbx), newStatus.Replicas, newStatus.Allocated, newStatus.Ready)
			if err := r.updateStatus(batchSbx, newStatus); err != nil {
				aggErrors = append(aggErrors, err)
			}
		}
	}

	// task schedule
	if batchSbx.Spec.TaskTemplate != nil {
		// Because tasks are in-memory and there is no event mechanism, periodic reconciliation is required.
		DurationStore.Push(types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String(), 3*time.Second)
		sch, err := r.getTaskScheduler(batchSbx, pods)
		if err != nil {
			return ctrl.Result{}, err
		}
		if batchSbx.DeletionTimestamp != nil {
			stoppingTasks := sch.StopTask()
			if len(stoppingTasks) > 0 {
				klog.Infof("BatchSandbox %s is stopping %d tasks this round", klog.KObj(batchSbx), len(stoppingTasks))
			}
		}
		now := time.Now()
		if err = r.scheduleTasks(ctx, sch, batchSbx); err != nil {
			klog.Errorf("BatchSandbox %s failed to schedule tasks, err %v", klog.KObj(batchSbx), err)
			aggErrors = append(aggErrors, err)
		} else {
			klog.Infof("BatchSandbox %s schedule tasks cost %d ms", klog.KObj(batchSbx), time.Since(now).Milliseconds())
		}
		// check task cleanup is finished
		if batchSbx.DeletionTimestamp != nil {
			unfinishedTasks := r.getTasksCleanupUnfinished(batchSbx, sch)
			if len(unfinishedTasks) > 0 {
				klog.Infof("BatchSandbox %s is terminating, tasks cleanup is unfinished, unfinished tasks %v", klog.KObj(batchSbx), unfinishedTasks)
			} else {
				var err error
				if controllerutil.ContainsFinalizer(batchSbx, FinalizerTaskCleanup) {
					err = utils.UpdateFinalizer(r.Client, batchSbx, utils.RemoveFinalizerOpType, FinalizerTaskCleanup)
					if err != nil {
						if errors.IsNotFound(err) {
							err = nil
						} else {
							klog.Errorf("failed to remove finalizer %s %s, err %v", FinalizerTaskCleanup, klog.KObj(batchSbx), err)
						}
					}
				}
				if err == nil {
					r.deleteTaskScheduler(batchSbx)
					klog.Infof("BatchSandbox %s is terminating, task cleanup is finished, remove finalizer %s %s", klog.KObj(batchSbx), FinalizerTaskCleanup, klog.KObj(batchSbx))
				}
				return ctrl.Result{}, err
			}
		}
	}

	return reconcile.Result{RequeueAfter: DurationStore.Pop(req.String())}, gerrors.Join(aggErrors...)
}

func (r *BatchSandboxReconciler) listPods(ctx context.Context, batchSbx *sandboxv1alpha1.BatchSandbox) ([]*corev1.Pod, error) {
	var ret []*corev1.Pod
	if batchSbx.Spec.PoolRef != "" {
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

func (r *BatchSandboxReconciler) getTaskScheduler(batchSbx *sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (taskscheduler.TaskScheduler, error) {
	var tSch taskscheduler.TaskScheduler
	key := types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String()
	val, ok := r.taskSchedulers.Load(key)
	// The reconciler guarantees that it will not concurrently reconcile the same BatchSandbox.
	if !ok {
		policy := sandboxv1alpha1.TaskResourcePolicyRetain
		if batchSbx.Spec.TaskResourcePolicyWhenCompleted != nil {
			policy = *batchSbx.Spec.TaskResourcePolicyWhenCompleted
		}
		taskSpecs, err := generaTaskSpec(batchSbx)
		if err != nil {
			return nil, err
		}
		sc, err := taskscheduler.NewTaskScheduler(key, taskSpecs, pods, policy)
		if err != nil {
			return nil, fmt.Errorf("new task scheduler err %w", err)
		}
		klog.Infof("successfully new task scheduler for batch sandbox %s", klog.KObj(batchSbx))
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

func (r *BatchSandboxReconciler) deleteTaskScheduler(batchSbx *sandboxv1alpha1.BatchSandbox) {
	klog.Infof("delete task scheduler for batch sandbox %s", klog.KObj(batchSbx))
	key := types.NamespacedName{Namespace: batchSbx.Namespace, Name: batchSbx.Name}.String()
	r.taskSchedulers.Delete(key)
}

func generaTaskSpec(batchSbx *sandboxv1alpha1.BatchSandbox) ([]*sandboxv1alpha1.Task, error) {
	ret := make([]*sandboxv1alpha1.Task, *batchSbx.Spec.Replicas)
	for idx := range int(*batchSbx.Spec.Replicas) {
		task, err := getTaskSpec(batchSbx, idx)
		if err != nil {
			return ret, err
		}
		ret[idx] = task
	}
	return ret, nil
}

func getTaskSpec(batchSbx *sandboxv1alpha1.BatchSandbox, idx int) (*sandboxv1alpha1.Task, error) {
	task := &sandboxv1alpha1.Task{
		ObjectMeta: batchSbx.Spec.TaskTemplate.ObjectMeta,
		Spec:       *batchSbx.Spec.TaskTemplate.Spec.DeepCopy(),
	}
	if task.Name == "" {
		task.Name = fmt.Sprintf("%s-%d", batchSbx.Name, idx)
	}
	if task.Namespace == "" {
		task.Namespace = batchSbx.Namespace
	}
	if len(batchSbx.Spec.ShardTaskPatches) > 0 && idx < len(batchSbx.Spec.ShardTaskPatches) {
		patch := batchSbx.Spec.ShardTaskPatches[idx]
		cloneBytes, _ := json.Marshal(task)
		modified, err := strategicpatch.StrategicMergePatch(cloneBytes, patch.Raw, &sandboxv1alpha1.Task{})
		if err != nil {
			return nil, fmt.Errorf("batchsandbox: failed to merge patch raw %s, idx %d, err %w", patch.Raw, idx, err)
		}
		newTask := &sandboxv1alpha1.Task{}
		if err = json.Unmarshal(modified, newTask); err != nil {
			return nil, fmt.Errorf("batchsandbox: failed to unmarshal %s to Task, idx %d, err %w", modified, idx, err)
		}
		*task = *newTask
	}
	return task, nil
}

func (r *BatchSandboxReconciler) scheduleTasks(ctx context.Context, tSch taskscheduler.TaskScheduler, batchSbx *sandboxv1alpha1.BatchSandbox) error {
	if err := tSch.Schedule(); err != nil {
		return err
	}
	tasks := tSch.ListTask()
	toReleasedPods := []string{}
	var (
		running, failed, succeed, unknown int32
		pending                           int32
	)
	for i := 0; i < len(tasks); i++ {
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
		klog.Infof("batch sandbox %s try to release %d Pod", klog.KObj(batchSbx), len(toReleasedPods))
		if err := r.releasePods(ctx, batchSbx, toReleasedPods); err != nil {
			return err
		}
		klog.Infof("batch sandbox %s successfully released %d Pods", klog.KObj(batchSbx), len(toReleasedPods))
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
		klog.Infof("To update BatchSandbox status for %s, replicas=%d task_running=%d task_succeed=%d, task_failed=%d, task_unknown=%d, task_pending=%d", klog.KObj(batchSbx), newStatus.Replicas,
			newStatus.TaskRunning, newStatus.TaskSucceed, newStatus.TaskFailed, newStatus.TaskUnknown, newStatus.TaskPending)
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
		klog.Infof("BatchSandbox %s scale expectation is not satisfied overtime=%v, dirty pods=%v", klog.KObj(batchSandbox), unsatisfiedDuration, dirtyPods)
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
		klog.Infof("BatchSandbox %s try to create %d Pod, idx %v", klog.KObj(batchSandbox), len(needCreateIndex), needCreateIndex)
	}
	for _, idx := range needCreateIndex {
		pod, err := utils.GetPodFromTemplate(podTemplateSpec, batchSandbox, metav1.NewControllerRef(batchSandbox, sandboxv1alpha1.SchemeBuilder.GroupVersion.WithKind("BatchSandbox")))
		if err != nil {
			return err
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
