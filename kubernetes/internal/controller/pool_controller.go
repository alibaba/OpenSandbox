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
	"math"
	"sort"
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

var (
	PoolScaleExpectations = expectations.NewScaleExpectations()
)

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
		if batchSandbox.Spec.Template != nil {
			continue
		}
		batchSandboxes = append(batchSandboxes, &batchSandbox)
	}
	log.Info("Pool reconcile", "pool", pool.Name, "pods", len(pods), "batchSandboxes", len(batchSandboxes))
	return r.reconcilePool(ctx, pool, batchSandboxes, pods)
}

// reconcilePool contains the main reconciliation logic
func (r *PoolReconciler) reconcilePool(ctx context.Context, pool *sandboxv1alpha1.Pool, batchSandboxes []*sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var result ctrl.Result

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// 1. Get latest Pool CR
		latestPool := &sandboxv1alpha1.Pool{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(pool), latestPool); err != nil {
			return err
		}

		// 2. Handle pod eviction requests
		allocBeforeSchedule, err := r.Allocator.GetPoolAllocation(ctx, latestPool)
		if err != nil {
			log.Error(err, "Failed to get pool allocation")
			return err
		}

		evictionErr := r.handlePodEvictions(ctx, latestPool, pods, allocBeforeSchedule)

		// 3. Filter out evicting pods before scheduling
		schedulePods := r.filterEvictingPods(ctx, latestPool, pods, allocBeforeSchedule)

		// 4. Schedule and allocate
		podAllocation, pendingSyncs, idlePods, dirtyPods, supplySandbox, poolDirty, err := r.scheduleSandbox(ctx, latestPool, batchSandboxes, schedulePods)
		if err != nil {
			return err
		}

		needReconcile := false
		delay := time.Duration(0)
		if supplySandbox > 0 && len(idlePods) > 0 {
			needReconcile = true
			delay = defaultRetryTime
		}
		if int32(len(idlePods)) >= supplySandbox {
			supplySandbox = 0
		} else {
			supplySandbox -= int32(len(idlePods))
		}

		// 1. Persist to memory
		if poolDirty {
			if err := r.Allocator.PersistPoolAllocation(ctx, latestPool, &AllocStatus{PodAllocation: podAllocation}); err != nil {
				log.Error(err, "Failed to persist pool allocation")
				return err
			}
		}

		// 2. Sync BatchSandbox allocations. Rollback in memory alocation if failed to sync.
		// Optimize this concurrently if needed.
		var syncErrs []error
		for _, syncInfo := range pendingSyncs {
			if err := r.Allocator.SyncSandboxAllocation(ctx, syncInfo.Sandbox, syncInfo.Pods); err != nil {
				log.Error(err, "Failed to sync sandbox allocation", "sandbox", syncInfo.SandboxName)
				syncErrs = append(syncErrs, fmt.Errorf("failed to sync sandbox %s: %w", syncInfo.SandboxName, err))
			} else {
				log.Info("Successfully sync Sandbox allocation", "sandbox", syncInfo.SandboxName, "pods", syncInfo.Pods)
			}
		}
		if len(syncErrs) > 0 {
			return gerrors.Join(syncErrs...)
		}

		latestRevision, err := r.calculateRevision(latestPool)
		if err != nil {
			return err
		}
		latestIdlePods, deleteOld, supplyNew := r.updatePool(ctx, latestRevision, schedulePods, idlePods, dirtyPods)

		args := &scaleArgs{
			latestRevision: latestRevision,
			pool:           latestPool,
			pods:           schedulePods,
			totalPodCnt:    int32(len(pods)),
			allocatedCnt:   int32(len(podAllocation)),
			idlePods:       latestIdlePods,
			redundantPods:  deleteOld,
			supplyCnt:      supplySandbox + supplyNew,
		}
		if err := r.scalePool(ctx, args); err != nil {
			return err
		}

		// 6. Update Status (use all pods for total count, schedulePods for available count)
		if err := r.updatePoolStatus(ctx, latestRevision, latestPool, pods, schedulePods, podAllocation); err != nil {
			return err
		}

		if needReconcile {
			result = ctrl.Result{RequeueAfter: delay}
		}

		// Return eviction error last to trigger requeue for failed evictions
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
func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		Complete(r)
}

func (r *PoolReconciler) scheduleSandbox(ctx context.Context, pool *sandboxv1alpha1.Pool, batchSandboxes []*sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (map[string]string, []SandboxSyncInfo, []string, []string, int32, bool, error) {
	log := logf.FromContext(ctx)
	spec := &AllocSpec{
		Sandboxes: batchSandboxes,
		Pool:      pool,
		Pods:      pods,
	}
	status, pendingSyncs, poolDirty, err := r.Allocator.Schedule(ctx, spec)
	if err != nil {
		return nil, nil, nil, nil, 0, false, err
	}
	idlePods := make([]string, 0)
	for _, pod := range pods {
		if _, ok := status.PodAllocation[pod.Name]; !ok {
			idlePods = append(idlePods, pod.Name)
		}
	}
	log.Info("Schedule result", "pool", pool.Name, "allocated", len(status.PodAllocation),
		"idlePods", len(idlePods), "supplement", status.PodSupplement, "pendingSyncs", len(pendingSyncs), "poolDirty", poolDirty)
	return status.PodAllocation, pendingSyncs, idlePods, status.DirtyPods, status.PodSupplement, poolDirty, nil
}

func (r *PoolReconciler) updatePool(ctx context.Context, latestRevision string, pods []*corev1.Pod, idlePods []string, dirtyPods []string) ([]string, []string, int32) {
	podMap := make(map[string]*corev1.Pod)
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}
	latestIdlePods := make([]string, 0)
	deleteOld := make([]string, 0)
	supplyNew := int32(0)

	dirtySet := make(map[string]bool)
	for _, p := range dirtyPods {
		dirtySet[p] = true
	}

	for _, name := range idlePods {
		if dirtySet[name] {
			deleteOld = append(deleteOld, name)
			// no need to supply, next reconcile will do this job
			continue
		}

		pod, ok := podMap[name]
		if !ok {
			continue
		}
		revision := pod.Labels[LabelPoolRevision]
		if revision == latestRevision {
			latestIdlePods = append(latestIdlePods, name)
		} else {
			// Rolling: (1) delete old idle pods (2) create latest pods
			deleteOld = append(deleteOld, name)
			supplyNew++
		}
	}
	if len(deleteOld) > 0 {
		logf.FromContext(ctx).Info("Rolling update detected", "latestRevision", latestRevision,
			"outdatedPods", deleteOld, "supplyNew", supplyNew, "latestIdlePods", len(latestIdlePods))
	}
	return latestIdlePods, deleteOld, supplyNew
}

type scaleArgs struct {
	latestRevision string
	pool           *sandboxv1alpha1.Pool
	pods           []*corev1.Pod
	totalPodCnt    int32 // all pods including evicting ones, for PoolMax enforcement
	allocatedCnt   int32
	supplyCnt      int32 // to create
	idlePods       []string
	redundantPods  []string
}

func (r *PoolReconciler) scalePool(ctx context.Context, args *scaleArgs) error {
	log := logf.FromContext(ctx)
	errs := make([]error, 0)
	pool := args.pool
	pods := args.pods
	if satisfied, unsatisfiedDuration, dirtyPods := PoolScaleExpectations.SatisfiedExpectations(controllerutils.GetControllerKey(pool)); !satisfied {
		log.Info("Pool scale is not ready, requeue", "unsatisfiedDuration", unsatisfiedDuration, "dirtyPods", dirtyPods)
		return fmt.Errorf("pool scale is not ready, %v", pool.Name)
	}
	schedulableCnt := int32(len(args.pods))
	totalPodCnt := args.totalPodCnt
	allocatedCnt := args.allocatedCnt
	supplyCnt := args.supplyCnt
	redundantPods := args.redundantPods
	bufferCnt := schedulableCnt - allocatedCnt

	// Calculate desired buffer cnt.
	desiredBufferCnt := bufferCnt
	if bufferCnt < pool.Spec.CapacitySpec.BufferMin || bufferCnt > pool.Spec.CapacitySpec.BufferMax {
		desiredBufferCnt = (pool.Spec.CapacitySpec.BufferMin + pool.Spec.CapacitySpec.BufferMax) / 2
	}

	// Calculate desired schedulable cnt.
	desiredSchedulableCnt := allocatedCnt + supplyCnt + desiredBufferCnt
	if desiredSchedulableCnt < pool.Spec.CapacitySpec.PoolMin {
		desiredSchedulableCnt = pool.Spec.CapacitySpec.PoolMin
	}
	// Enforce PoolMax: limit new pods based on total running pods (including evicting).
	maxNewPods := pool.Spec.CapacitySpec.PoolMax - totalPodCnt
	if maxNewPods < 0 {
		maxNewPods = 0
	}

	log.Info("Scale pool decision", "pool", pool.Name,
		"totalPodCnt", totalPodCnt, "schedulableCnt", schedulableCnt,
		"allocatedCnt", allocatedCnt, "bufferCnt", bufferCnt,
		"desiredBufferCnt", desiredBufferCnt, "supplyCnt", supplyCnt,
		"desiredSchedulableCnt", desiredSchedulableCnt, "maxNewPods", maxNewPods,
		"redundantPods", len(redundantPods), "idlePods", len(args.idlePods))

	// Scale-up: create new pods if needed and allowed by PoolMax
	if desiredSchedulableCnt > schedulableCnt && maxNewPods > 0 {
		createCnt := desiredSchedulableCnt - schedulableCnt
		if createCnt > maxNewPods {
			createCnt = maxNewPods
		}
		maxUnavailable := r.getMaxUnavailable(pool, desiredSchedulableCnt)
		notReadyCnt := r.countNotReadyPods(pods)
		limitedCreatCnt := maxUnavailable - notReadyCnt
		createCnt = int32(math.Max(0, math.Min(float64(createCnt), float64(limitedCreatCnt))))
		if createCnt > 0 {
			log.Info("Scaling up pool with constraint", "pool", pool.Name,
				"createCnt", createCnt, "maxUnavailable", maxUnavailable,
				"notReadyCnt", notReadyCnt, "desiredSchedulableCnt", desiredSchedulableCnt, "limitedCreatCnt", limitedCreatCnt)
			for range createCnt {
				if err := r.createPoolPod(ctx, pool, args.latestRevision); err != nil {
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
	if scaleIn > 0 || len(redundantPods) > 0 {
		podsToDelete := r.pickPodsToDelete(pods, args.idlePods, args.redundantPods, scaleIn)
		log.Info("Scaling down pool", "pool", pool.Name, "scaleIn", scaleIn, "redundantPods", len(redundantPods), "podsToDelete", len(podsToDelete))
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

func (r *PoolReconciler) updatePoolStatus(ctx context.Context, latestRevision string, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod, schedulePods []*corev1.Pod, podAllocation map[string]string) error {
	oldStatus := pool.Status.DeepCopy()
	availableCnt := int32(0)
	for _, pod := range schedulePods {
		if _, ok := podAllocation[pod.Name]; ok {
			continue
		}
		if !isPodReady(pod) {
			continue
		}
		availableCnt++
	}
	pool.Status.ObservedGeneration = pool.Generation
	pool.Status.Total = int32(len(pods))
	pool.Status.Allocated = int32(len(podAllocation))
	pool.Status.Available = availableCnt
	pool.Status.Revision = latestRevision
	if equality.Semantic.DeepEqual(oldStatus, pool.Status) {
		return nil
	}
	log := logf.FromContext(ctx)
	log.Info("Update pool status", "ObservedGeneration", pool.Status.ObservedGeneration, "Total", pool.Status.Total,
		"Allocated", pool.Status.Allocated, "Available", pool.Status.Available, "Revision", pool.Status.Revision)
	if err := r.Status().Update(ctx, pool); err != nil {
		return err
	}
	return nil
}

func (r *PoolReconciler) pickPodsToDelete(pods []*corev1.Pod, idlePodNames []string, redundantPodNames []string, scaleIn int32) []*corev1.Pod {
	var idlePods []*corev1.Pod
	podMap := make(map[string]*corev1.Pod)
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}
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
	var podsToDelete []*corev1.Pod
	for _, name := range redundantPodNames { // delete pod from pool update
		pod, ok := podMap[name]
		if !ok {
			continue
		}
		podsToDelete = append(podsToDelete, pod)
	}
	for _, pod := range idlePods { // delete pod from pool scale
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

// getMaxUnavailable returns the resolved maxUnavailable value.
// If not specified, defaults to 25% of desiredTotal.
// Minimum return value is 1 to ensure scaling progress.
func (r *PoolReconciler) getMaxUnavailable(pool *sandboxv1alpha1.Pool, desiredTotal int32) int32 {
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
		if !isPodReady(pod) {
			count++
		}
	}
	return count
}

func (r *PoolReconciler) createPoolPod(ctx context.Context, pool *sandboxv1alpha1.Pool, latestRevision string) error {
	log := logf.FromContext(ctx)
	pod, err := utils.GetPodFromTemplate(pool.Spec.Template, pool, metav1.NewControllerRef(pool, sandboxv1alpha1.SchemeBuilder.GroupVersion.WithKind("Pool")))
	if err != nil {
		return err
	}
	pod.Namespace = pool.Namespace
	pod.Name = ""
	pod.GenerateName = pool.Name + "-"
	pod.Labels[LabelPoolName] = pool.Name
	pod.Labels[LabelPoolRevision] = latestRevision
	if err := ctrl.SetControllerReference(pool, pod, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, pod); err != nil {
		r.Recorder.Eventf(pool, corev1.EventTypeWarning, "FailedCreate", "Failed to create pool pod: %v", err)
		return err
	}
	PoolScaleExpectations.ExpectScale(controllerutils.GetControllerKey(pool), expectations.Create, pod.Name)
	log.Info("Created pool pod", "pool", pool.Name, "pod", pod.Name, "revision", latestRevision)
	r.Recorder.Eventf(pool, corev1.EventTypeNormal, "SuccessfulCreate", "Created pool pod: %v", pod.Name)
	return nil
}

// handlePodEvictions evicts idle pods marked for eviction.
// Eviction errors don't block the current reconcile; they are returned last to trigger requeue.
func (r *PoolReconciler) handlePodEvictions(ctx context.Context, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod, podAllocation map[string]string) error {
	log := logf.FromContext(ctx)

	handler := eviction.NewEvictionHandler(ctx, r.Client, pool)

	var evictionErrs []error
	for _, pod := range pods {
		if !handler.NeedsEviction(pod) {
			continue
		}

		if sandboxName, allocated := podAllocation[pod.Name]; allocated {
			log.V(1).Info("Skipping eviction for allocated pod", "pod", pod.Name, "sandbox", sandboxName)
			continue
		}

		log.Info("Evicting idle pool pod", "pool", pool.Name, "pod", pod.Name)
		if err := handler.Evict(ctx, pod); err != nil {
			log.Error(err, "Failed to evict pod", "pod", pod.Name)
			evictionErrs = append(evictionErrs, fmt.Errorf("failed to evict pod %s: %w", pod.Name, err))
		} else {
			r.Recorder.Eventf(pool, corev1.EventTypeNormal, "PodEvicted", "Evicted idle pod: %s", pod.Name)
		}
	}

	return gerrors.Join(evictionErrs...)
}

// filterEvictingPods excludes idle pods marked for eviction from scheduling candidates.
// Allocated pods with eviction label are kept because they won't be deleted.
func (r *PoolReconciler) filterEvictingPods(ctx context.Context, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod, podAllocation map[string]string) []*corev1.Pod {
	handler := eviction.NewEvictionHandler(ctx, r.Client, pool)
	filtered := make([]*corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if handler.NeedsEviction(pod) && podAllocation[pod.Name] == "" {
			continue
		}
		filtered = append(filtered, pod)
	}
	return filtered
}

func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
