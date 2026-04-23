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
	"crypto/sha256"
	"encoding/hex"
	gerrors "errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/controller/eviction"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils"
	controllerutils "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/controller"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/expectations"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/fieldindex"
)

const (
	defaultRetryTime = 5 * time.Second
)

const (
	LabelPoolName     = "sandbox.opensandbox.io/pool-name"
	LabelPoolRevision = "sandbox.opensandbox.io/pool-revision"
)

const (
	defaultSyncSandboxAllocConcurrency = 256
	envSyncSandboxAllocConcurrency     = "SYNC_SANDBOX_ALLOC_CONCURRENCY"
)

var (
	PoolScaleExpectations       = expectations.NewScaleExpectations()
	syncSandboxAllocConcurrency int
)

func init() {
	syncSandboxAllocConcurrency = defaultSyncSandboxAllocConcurrency
	if val := os.Getenv(envSyncSandboxAllocConcurrency); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			syncSandboxAllocConcurrency = n
		}
	}
}

// PoolReconciler reconciles a Pool object
type PoolReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Allocator Allocator
}

// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=pools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=pools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=pools/finalizers,verbs=update
// +kubebuilder:rbac:groups=sandbox.opensandbox.io,resources=batchsandboxes,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *PoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	// Fetch the Pool instance
	pool := &sandboxv1alpha1.Pool{}
	if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
		if errors.IsNotFound(err) {
			// Pool resource not found, could have been deleted
			controllerKey := req.NamespacedName.String()
			PoolScaleExpectations.DeleteExpectations(controllerKey)
			r.Allocator.ClearPoolAllocation(ctx, req.Namespace, req.Name)
			log.Info("Pool resource not found, cleaned up scale expectations", "pool", controllerKey)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.Error(err, "Failed to get Pool")
		return ctrl.Result{}, err
	}
	if !pool.DeletionTimestamp.IsZero() {
		controllerKey := controllerutils.GetControllerKey(pool)
		PoolScaleExpectations.DeleteExpectations(controllerKey)
		r.Allocator.ClearPoolAllocation(ctx, req.Namespace, req.Name)
		log.Info("Pool resource is being deleted, cleaned up scale expectations", "pool", controllerKey)
		return ctrl.Result{}, nil
	}

	// List all pods of the pool
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, &client.ListOptions{
		Namespace:     pool.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(pool.UID)}),
	}); err != nil {
		log.Error(err, "Failed to list pods")
		return reconcile.Result{}, err
	}
	pods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := podList.Items[i]
		PoolScaleExpectations.ObserveScale(controllerutils.GetControllerKey(pool), expectations.Create, pod.Name)
		if pod.DeletionTimestamp.IsZero() {
			pods = append(pods, &pod)
		}
	}

	// List all batch sandboxes  ref to the pool
	batchSandboxList := &sandboxv1alpha1.BatchSandboxList{}
	if err := r.List(ctx, batchSandboxList, &client.ListOptions{
		Namespace:     pool.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForPoolRef: pool.Name}),
	}); err != nil {
		log.Error(err, "Failed to list batch sandboxes")
		return reconcile.Result{}, err
	}
	batchSandboxes := make([]*sandboxv1alpha1.BatchSandbox, 0, len(batchSandboxList.Items))
	for i := range batchSandboxList.Items {
		batchSandbox := batchSandboxList.Items[i]
		if !shouldManageBatchSandboxInPool(&batchSandbox) {
			continue
		}
		batchSandboxes = append(batchSandboxes, &batchSandbox)
	}
	log.Info("Pool reconcile", "pool", pool.Name, "pods", len(pods), "batchSandboxes", len(batchSandboxes))
	return r.reconcilePool(ctx, pool, batchSandboxes, pods)
}

func shouldManageBatchSandboxInPool(batchSandbox *sandboxv1alpha1.BatchSandbox) bool {
	if batchSandbox == nil {
		return false
	}
	if batchSandbox.Spec.PoolRef == "" {
		return false
	}
	// Pause solidifies the pool template onto the BatchSandbox before the snapshot commits.
	// Keep the sandbox visible to pool scheduling until resume detaches it by clearing poolRef,
	// otherwise the source pool pod can be GC'd and recycled mid-snapshot.
	return true
}

// reconcilePool contains the main reconciliation logic
func (r *PoolReconciler) reconcilePool(ctx context.Context, pool *sandboxv1alpha1.Pool, batchSandboxes []*sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (ctrl.Result, error) {
	var result ctrl.Result

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// 1. Get latest Pool CR
		latestPool := &sandboxv1alpha1.Pool{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(pool), latestPool); err != nil {
			return err
		}

		// 2. Handle pod eviction
		schedulePods, evictionErr := r.handleEviction(ctx, latestPool, pods)
		if schedulePods == nil {
			return evictionErr
		}

		// 3. Schedule sandbox (compute + persist + sync)
		schedResult, err := r.scheduleSandbox(ctx, latestPool, batchSandboxes, schedulePods)
		if err != nil {
			return err
		}
		// Requeue if there are pending sandboxes waiting for scheduling
		if schedResult.SupplyCnt > 0 {
			result = ctrl.Result{RequeueAfter: defaultRetryTime}
		}

		// 4. Handle pool upgrade
		updateResult, err := r.updatePool(ctx, latestPool, schedulePods, schedResult.IdlePods)
		if err != nil {
			return err
		}

		// 5. Handle pool scale
		toDeletePods := append(updateResult.ToDeletePods, schedResult.DirtyPods...)
		args := &scaleArgs{
			updateRevision: updateResult.UpdateRevision,
			pods:           schedulePods,
			totalPodCnt:    int32(len(pods)),
			allocatedCnt:   int32(len(schedResult.PodAllocation)),
			idlePods:       updateResult.IdlePods,
			toDeletePods:   toDeletePods,
			supplyCnt:      schedResult.SupplyCnt + updateResult.SupplyUpdateRevision,
		}

		if err := r.scalePool(ctx, latestPool, args); err != nil {
			return err
		}

		// 6. Update pool status
		if err := r.updatePoolStatus(ctx, updateResult.UpdateRevision, latestPool, pods, schedulePods, schedResult.PodAllocation); err != nil {
			return err
		}

		if evictionErr != nil {
			return evictionErr
		}

		return nil
	})

	return result, err
}

func (r *PoolReconciler) calculateRevision(pool *sandboxv1alpha1.Pool) (string, error) {
	template, err := json.Marshal(pool.Spec.Template)
	if err != nil {
		return "", err
	}
	revision := sha256.Sum256(template)
	return hex.EncodeToString(revision[:8]), nil
}

// SetupWithManager sets up the controller with the Manager.
// Todo pod deletion expectations
func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int) error {
	filterBatchSandbox := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			bsb, ok := e.Object.(*sandboxv1alpha1.BatchSandbox)
			if !ok {
				return false
			}
			return bsb.Spec.PoolRef != ""
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, okOld := e.ObjectOld.(*sandboxv1alpha1.BatchSandbox)
			newObj, okNew := e.ObjectNew.(*sandboxv1alpha1.BatchSandbox)
			if !okOld || !okNew {
				return false
			}
			if newObj.Spec.PoolRef == "" {
				return false
			}
			oldVal := oldObj.Annotations[AnnoAllocReleaseKey]
			newVal := newObj.Annotations[AnnoAllocReleaseKey]
			if oldVal != newVal {
				return true
			}
			if oldObj.Spec.Replicas != newObj.Spec.Replicas {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			bsb, ok := e.Object.(*sandboxv1alpha1.BatchSandbox)
			if !ok {
				return false
			}
			return bsb.Spec.PoolRef != ""
		},
		GenericFunc: func(e event.GenericEvent) bool {
			bsb, ok := e.Object.(*sandboxv1alpha1.BatchSandbox)
			if !ok {
				return false
			}
			return bsb.Spec.PoolRef != ""
		},
	}

	findPoolForBatchSandbox := func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		batchSandbox, ok := obj.(*sandboxv1alpha1.BatchSandbox)
		if !ok {
			log.Error(nil, "Invalid object type, expected BatchSandbox")
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: batchSandbox.Namespace,
					Name:      batchSandbox.Spec.PoolRef,
				},
			},
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&sandboxv1alpha1.Pool{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Pod{}).
		Watches(
			&sandboxv1alpha1.BatchSandbox{},
			handler.EnqueueRequestsFromMapFunc(findPoolForBatchSandbox),
			builder.WithPredicates(filterBatchSandbox),
		).
		Named("pool").
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconciles}).
		Complete(r)
}

func (r *PoolReconciler) scheduleSandbox(ctx context.Context, pool *sandboxv1alpha1.Pool, batchSandboxes []*sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (*ScheduleResult, error) {
	log := logf.FromContext(ctx)
	spec := &AllocSpec{
		Sandboxes: batchSandboxes,
		Pool:      pool,
		Pods:      pods,
	}
	allocStatus, pendingSyncs, poolDirty, err := r.Allocator.Schedule(ctx, spec)
	if err != nil {
		return nil, err
	}
	idlePods := make([]string, 0)
	for _, pod := range pods {
		if _, ok := allocStatus.PodAllocation[pod.Name]; !ok {
			idlePods = append(idlePods, pod.Name)
		}
	}
	log.Info("Schedule result", "pool", pool.Name, "allocated", len(allocStatus.PodAllocation),
		"idlePods", len(idlePods), "supplement", allocStatus.PodSupplement, "pendingSyncs", len(pendingSyncs), "poolDirty", poolDirty)

	schedResult := &ScheduleResult{
		PodAllocation: allocStatus.PodAllocation,
		IdlePods:      idlePods,
		DirtyPods:     allocStatus.DirtyPods,
		SupplyCnt:     allocStatus.PodSupplement,
	}

	// Persist allocation to memory store
	if poolDirty {
		if err := r.Allocator.PersistPoolAllocation(ctx, pool, &AllocStatus{PodAllocation: allocStatus.PodAllocation}); err != nil {
			log.Error(err, "Failed to persist pool allocation")
			return nil, err
		}
	}

	// Sync to each BatchSandbox concurrently
	errCh := make(chan error, len(pendingSyncs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, syncSandboxAllocConcurrency)

	for _, syncInfo := range pendingSyncs {
		wg.Add(1)
		sem <- struct{}{}
		go func(info SandboxSyncInfo) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := r.Allocator.SyncSandboxAllocation(ctx, info.Sandbox, info.Pods); err != nil {
				log.Error(err, "Failed to sync sandbox allocation", "sandbox", info.SandboxName)
				errCh <- fmt.Errorf("failed to sync sandbox %s: %w", info.SandboxName, err)
			} else {
				log.Info("Successfully sync Sandbox allocation", "sandbox", info.SandboxName, "pods", info.Pods)
			}
		}(syncInfo)
	}
	wg.Wait()
	close(errCh)

	var syncErrs []error
	for err := range errCh {
		syncErrs = append(syncErrs, err)
	}

	if err := gerrors.Join(syncErrs...); err != nil {
		return nil, err
	}

	return schedResult, nil
}

func (r *PoolReconciler) updatePool(ctx context.Context, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod, idlePods []string) (*UpdateResult, error) {
	updateRevision, err := r.calculateRevision(pool)
	if err != nil {
		return nil, err
	}
	strategy := NewPoolUpdateStrategy(pool)
	result := strategy.Compute(ctx, updateRevision, pods, idlePods)
	result.UpdateRevision = updateRevision
	return result, nil
}

type scaleArgs struct {
	updateRevision string
	pods           []*corev1.Pod
	totalPodCnt    int32 // all pods including evicting ones, for PoolMax enforcement
	allocatedCnt   int32
	supplyCnt      int32 // to create
	idlePods       []string
	toDeletePods   []string
}

// ScheduleResult holds the output of scheduleSandbox.
type ScheduleResult struct {
	PodAllocation map[string]string
	IdlePods      []string
	DirtyPods     []string
	SupplyCnt     int32
}

type UpdateResult struct {
	UpdateRevision string
	IdlePods       []string
	ToDeletePods   []string
	// Supply Pods with update revision
	SupplyUpdateRevision int32
}

func (r *PoolReconciler) scalePool(ctx context.Context, pool *sandboxv1alpha1.Pool, args *scaleArgs) error {
	log := logf.FromContext(ctx)
	errs := make([]error, 0)
	pods := args.pods
	if satisfied, unsatisfiedDuration, dirtyPods := PoolScaleExpectations.SatisfiedExpectations(controllerutils.GetControllerKey(pool)); !satisfied {
		log.Info("Pool scale is not ready, requeue", "unsatisfiedDuration", unsatisfiedDuration, "dirtyPods", dirtyPods)
		return fmt.Errorf("pool scale is not ready, %v", pool.Name)
	}
	schedulableCnt := int32(len(args.pods))
	totalPodCnt := args.totalPodCnt
	allocatedCnt := args.allocatedCnt
	supplyCnt := args.supplyCnt
	toDeletePods := args.toDeletePods
	bufferCnt := schedulableCnt - allocatedCnt

	// Calculate desired buffer cnt.
	desiredBufferCnt := bufferCnt
	if bufferCnt < pool.Spec.CapacitySpec.BufferMin || bufferCnt > pool.Spec.CapacitySpec.BufferMax {
		desiredBufferCnt = (pool.Spec.CapacitySpec.BufferMin + pool.Spec.CapacitySpec.BufferMax) / 2
	}

	// Calculate desired schedulable cnt.
	desiredSchedulableCnt := max(allocatedCnt+supplyCnt+desiredBufferCnt, pool.Spec.CapacitySpec.PoolMin)
	// Enforce PoolMax: limit new pods based on total running pods (including evicting).
	maxNewPods := max(pool.Spec.CapacitySpec.PoolMax-totalPodCnt, 0)

	log.Info("Scale pool decision", "pool", pool.Name,
		"totalPodCnt", totalPodCnt, "schedulableCnt", schedulableCnt,
		"allocatedCnt", allocatedCnt, "bufferCnt", bufferCnt,
		"desiredBufferCnt", desiredBufferCnt, "supplyCnt", supplyCnt,
		"desiredSchedulableCnt", desiredSchedulableCnt, "maxNewPods", maxNewPods,
		"toDeletePods", len(toDeletePods), "idlePods", len(args.idlePods))

	// Scale-up: create new pods if needed and allowed by PoolMax
	if desiredSchedulableCnt > schedulableCnt && maxNewPods > 0 {
		createCnt := min(desiredSchedulableCnt-schedulableCnt, maxNewPods)
		scaleMaxUnavailable := r.getScaleMaxUnavailable(pool, desiredSchedulableCnt)
		notReadyCnt := r.countNotReadyPods(pods)
		limitedCreateCnt := scaleMaxUnavailable - notReadyCnt
		createCnt = max(0, min(createCnt, limitedCreateCnt))
		if createCnt > 0 {
			log.Info("Scaling up pool with constraint", "pool", pool.Name,
				"createCnt", createCnt, "scaleMaxUnavailable", scaleMaxUnavailable,
				"notReadyCnt", notReadyCnt, "desiredSchedulableCnt", desiredSchedulableCnt, "limitedCreateCnt", limitedCreateCnt)
			for range createCnt {
				if err := r.createPoolPod(ctx, pool, args.updateRevision); err != nil {
					log.Error(err, "Failed to create pool pod")
					errs = append(errs, err)
				}
			}
		}
	}

	// Scale-down: delete redundant or excess pods
	scaleIn := int32(0)
	if desiredSchedulableCnt < schedulableCnt {
		scaleIn = schedulableCnt - desiredSchedulableCnt
	}
	if scaleIn > 0 || len(toDeletePods) > 0 {
		podsToDelete := r.pickPodsToDelete(pods, args.idlePods, args.toDeletePods, scaleIn)
		log.Info("Scaling down pool", "pool", pool.Name, "scaleIn", scaleIn, "toDeletePods", len(toDeletePods), "podsToDelete", len(podsToDelete))
		for _, pod := range podsToDelete {
			log.Info("Deleting pool pod", "pool", pool.Name, "pod", pod.Name)
			if err := r.Delete(ctx, pod); err != nil {
				log.Error(err, "Failed to delete pool pod", "pod", pod.Name)
				errs = append(errs, err)
			}
		}
	}
	return gerrors.Join(errs...)
}

func (r *PoolReconciler) updatePoolStatus(ctx context.Context, updateRevision string, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod, schedulePods []*corev1.Pod, podAllocation map[string]string) error {
	oldStatus := pool.Status.DeepCopy()
	availableCnt := int32(0)
	for _, pod := range schedulePods {
		if _, ok := podAllocation[pod.Name]; ok {
			continue
		}
		if !utils.IsPodReady(pod) {
			continue
		}
		availableCnt++
	}
	updatedCnt := int32(0)
	for _, pod := range pods {
		if pod.Labels[LabelPoolRevision] == updateRevision {
			updatedCnt++
		}
	}
	pool.Status.ObservedGeneration = pool.Generation
	pool.Status.Total = int32(len(pods))
	pool.Status.Allocated = int32(len(podAllocation))
	pool.Status.Available = availableCnt
	pool.Status.Revision = updateRevision
	pool.Status.Updated = updatedCnt
	if equality.Semantic.DeepEqual(*oldStatus, pool.Status) {
		return nil
	}
	log := logf.FromContext(ctx)
	log.Info("Update pool status", "ObservedGeneration", pool.Status.ObservedGeneration, "Total", pool.Status.Total,
		"Allocated", pool.Status.Allocated, "Available", pool.Status.Available, "Revision", pool.Status.Revision, "Updated", pool.Status.Updated)
	if err := r.Status().Update(ctx, pool); err != nil {
		return err
	}
	return nil
}

func (r *PoolReconciler) pickPodsToDelete(pods []*corev1.Pod, idlePodNames []string, toDeletePodNames []string, scaleIn int32) []*corev1.Pod {
	podMap := make(map[string]*corev1.Pod)
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}

	var podsToDelete []*corev1.Pod
	for _, name := range toDeletePodNames {
		pod, ok := podMap[name]
		if !ok {
			continue
		}
		podsToDelete = append(podsToDelete, pod)
	}

	var idlePods []*corev1.Pod
	for _, name := range idlePodNames {
		pod, ok := podMap[name]
		if !ok {
			continue
		}
		idlePods = append(idlePods, pod)
	}
	sort.Slice(idlePods, func(i, j int) bool {
		return idlePods[i].CreationTimestamp.Before(&idlePods[j].CreationTimestamp)
	})
	for _, pod := range idlePods {
		if scaleIn <= 0 {
			break
		}
		if pod.DeletionTimestamp == nil {
			podsToDelete = append(podsToDelete, pod)
		}
		scaleIn -= 1
	}
	return podsToDelete
}

// getScaleMaxUnavailable returns the resolved maxUnavailable value.
// If not specified, defaults to 25% of desiredTotal.
// Minimum return value is 1 to ensure scaling progress.
func (r *PoolReconciler) getScaleMaxUnavailable(pool *sandboxv1alpha1.Pool, desiredTotal int32) int32 {
	defaultPercentage := intstr.FromString("25%")

	maxUnavailable := &defaultPercentage
	if pool.Spec.ScaleStrategy != nil && pool.Spec.ScaleStrategy.MaxUnavailable != nil {
		maxUnavailable = pool.Spec.ScaleStrategy.MaxUnavailable
	}

	result, err := intstr.GetScaledValueFromIntOrPercent(maxUnavailable, int(desiredTotal), true)
	if err != nil || result < 1 {
		result = 1
	}
	return int32(result)
}

// countNotReadyPods returns the count of pods that are not ready.
// A pod is considered not ready if it doesn't have a Ready condition
// with status True.
func (r *PoolReconciler) countNotReadyPods(pods []*corev1.Pod) int32 {
	var count int32
	for _, pod := range pods {
		if !utils.IsPodReady(pod) {
			count++
		}
	}
	return count
}

func (r *PoolReconciler) createPoolPod(ctx context.Context, pool *sandboxv1alpha1.Pool, updateRevision string) error {
	log := logf.FromContext(ctx)
	pod, err := utils.GetPodFromTemplate(pool.Spec.Template, pool, metav1.NewControllerRef(pool, sandboxv1alpha1.SchemeBuilder.GroupVersion.WithKind("Pool")))
	if err != nil {
		return err
	}
	pod.Namespace = pool.Namespace
	pod.Name = ""
	pod.GenerateName = pool.Name + "-"
	pod.Labels[LabelPoolName] = pool.Name
	pod.Labels[LabelPoolRevision] = updateRevision
	if err := ctrl.SetControllerReference(pool, pod, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, pod); err != nil {
		r.Recorder.Eventf(pool, corev1.EventTypeWarning, "FailedCreate", "Failed to create pool pod: %v", err)
		return err
	}
	PoolScaleExpectations.ExpectScale(controllerutils.GetControllerKey(pool), expectations.Create, pod.Name)
	log.Info("Created pool pod", "pool", pool.Name, "pod", pod.Name, "revision", updateRevision)
	r.Recorder.Eventf(pool, corev1.EventTypeNormal, "SuccessfulCreate", "Created pool pod: %v", pod.Name)
	return nil
}

// handleEviction fetches the current allocation, evicts idle pods marked for eviction,
// and returns the schedulable pods (excluding evicting idle pods) along with any eviction error.
// Eviction errors are non-fatal: they are returned to trigger a requeue but do not block the current reconcile.
func (r *PoolReconciler) handleEviction(ctx context.Context, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod) ([]*corev1.Pod, error) {
	log := logf.FromContext(ctx)

	podAllocation, err := r.Allocator.GetPoolAllocation(ctx, pool)
	if err != nil {
		log.Error(err, "Failed to get pool allocation")
		return nil, err
	}

	handler := eviction.NewEvictionHandler(ctx, r.Client, pool)

	var evictionErrs []error
	filtered := make([]*corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if !handler.NeedsEviction(pod) {
			filtered = append(filtered, pod)
			continue
		}

		if sandboxName, allocated := podAllocation[pod.Name]; allocated {
			log.V(1).Info("Skipping eviction for allocated pod", "pod", pod.Name, "sandbox", sandboxName)
			filtered = append(filtered, pod)
			continue
		}

		// Idle pod marked for eviction: evict and exclude from scheduling
		log.Info("Evicting idle pool pod", "pool", pool.Name, "pod", pod.Name)
		if err := handler.Evict(ctx, pod); err != nil {
			log.Error(err, "Failed to evict pod", "pod", pod.Name)
			evictionErrs = append(evictionErrs, fmt.Errorf("failed to evict pod %s: %w", pod.Name, err))
		} else {
			r.Recorder.Eventf(pool, corev1.EventTypeNormal, "PodEvicted", "Evicted idle pod: %s", pod.Name)
		}
	}

	return filtered, gerrors.Join(evictionErrs...)
}
