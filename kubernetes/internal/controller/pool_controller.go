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
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/api/v1alpha1"
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
			log.Info("Pool resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.Error(err, "Failed to get Pool")
		return ctrl.Result{}, err
	}
	if !pool.DeletionTimestamp.IsZero() {
		log.Info("Pool resource is being deleted, ignoring")
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
	} // Main reconciliation logic
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

		// 2. Schedule and allocate
		podAllocation, idlePods, supplySandbox, poolDirty, err := r.scheduleSandbox(ctx, latestPool, batchSandboxes, pods)
		if err != nil {
			return err
		}

		needReconcile := false
		delay := time.Duration(0)
		if supplySandbox > 0 && len(idlePods) > 0 { // Some idle pods may be pending, retry schedule later.
			needReconcile = true
			delay = defaultRetryTime
		}
		if int32(len(idlePods)) >= supplySandbox { // Some pods may be pending, no need to create again.
			supplySandbox = 0
		} else {
			supplySandbox -= int32(len(idlePods))
		}

		// 3. Persist allocation if needed (Update Annotations)
		if poolDirty {
			if err := r.Allocator.PersistPoolAllocation(ctx, latestPool, &AllocStatus{PodAllocation: podAllocation}); err != nil {
				log.Error(err, "Failed to persist pool allocation")
				return err
			}
		}

		// 4. Update revision and scale (Scaling involves Pod creation/deletion, not Pool CR update)
		latestRevision, err := r.calculateRevision(latestPool)
		if err != nil {
			return err
		}
		latestIdlePods, deleteOld, supplyNew := r.updatePool(latestRevision, pods, idlePods)

		args := &scaleArgs{
			latestRevision: latestRevision,
			pool:           latestPool,
			pods:           pods,
			allocatedCnt:   int32(len(podAllocation)),
			idlePods:       latestIdlePods,
			redundantPods:  deleteOld,
			supplyCnt:      supplySandbox + supplyNew,
		}
		if err := r.scalePool(ctx, args); err != nil {
			return err
		}

		// 5. Update Status (using latestPool which has updated ResourceVersion)
		if err := r.updatePoolStatus(ctx, latestRevision, latestPool, pods, podAllocation); err != nil {
			return err
		}

		if needReconcile {
			result = ctrl.Result{RequeueAfter: delay}
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

func (r *PoolReconciler) scheduleSandbox(ctx context.Context, pool *sandboxv1alpha1.Pool, batchSandboxes []*sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (map[string]string, []string, int32, bool, error) {
	spec := &AllocSpec{
		Sandboxes: batchSandboxes,
		Pool:      pool,
		Pods:      pods,
	}
	status, poolDirty, err := r.Allocator.Schedule(ctx, spec)
	if err != nil {
		return nil, nil, 0, false, err
	}
	idlePods := make([]string, 0)
	for _, pod := range pods {
		if _, ok := status.PodAllocation[pod.Name]; !ok {
			idlePods = append(idlePods, pod.Name)
		}
	}
	return status.PodAllocation, idlePods, status.PodSupplement, poolDirty, nil
}

func (r *PoolReconciler) updatePool(latestRevision string, pods []*corev1.Pod, idlePods []string) ([]string, []string, int32) {
	podMap := make(map[string]*corev1.Pod)
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}
	latestIdlePods := make([]string, 0)
	deleteOld := make([]string, 0)
	supplyNew := int32(0)

	for _, name := range idlePods {
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
	return latestIdlePods, deleteOld, supplyNew
}

type scaleArgs struct {
	latestRevision string
	pool           *sandboxv1alpha1.Pool
	pods           []*corev1.Pod
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
	totalCnt := int32(len(args.pods))
	allocatedCnt := args.allocatedCnt
	supplyCnt := args.supplyCnt
	redundantPods := args.redundantPods
	bufferCnt := totalCnt - allocatedCnt

	// Calculate desired buffer cnt.
	desiredBufferCnt := bufferCnt
	if bufferCnt < pool.Spec.CapacitySpec.BufferMin || bufferCnt > pool.Spec.CapacitySpec.BufferMax {
		desiredBufferCnt = (pool.Spec.CapacitySpec.BufferMin + pool.Spec.CapacitySpec.BufferMax) / 2
	}

	// Calculate desired total cnt.
	desiredTotalCnt := allocatedCnt + supplyCnt + desiredBufferCnt
	if desiredTotalCnt < pool.Spec.CapacitySpec.PoolMin {
		desiredTotalCnt = pool.Spec.CapacitySpec.PoolMin
	} else if desiredTotalCnt > pool.Spec.CapacitySpec.PoolMax {
		desiredTotalCnt = pool.Spec.CapacitySpec.PoolMax
	}

	if desiredTotalCnt > totalCnt { // Need to create pod
		createCnt := desiredTotalCnt - totalCnt
		for i := int32(0); i < createCnt; i++ {
			if err := r.createPoolPod(ctx, pool, args.latestRevision); err != nil {
				log.Error(err, "Failed to create pool pod")
				errs = append(errs, err)
			}
		}
	} else if desiredTotalCnt < totalCnt || len(redundantPods) > 0 { // Need to delete pod
		scaleIn := int32(0)
		if desiredTotalCnt < totalCnt {
			scaleIn = totalCnt - desiredTotalCnt
		}
		podsToDelete := r.pickPodsToDelete(pods, args.idlePods, args.redundantPods, scaleIn)
		for _, pod := range podsToDelete {
			if err := r.Delete(ctx, pod); err != nil {
				log.Error(err, "Failed to delete pool pod")
				errs = append(errs, err)
			}
		}
	}
	return gerrors.Join(errs...)
}

func (r *PoolReconciler) updatePoolStatus(ctx context.Context, latestRevision string, pool *sandboxv1alpha1.Pool, pods []*corev1.Pod, podAllocation map[string]string) error {
	oldStatus := pool.Status.DeepCopy()
	availableCnt := int32(0)
	for _, pod := range pods {
		if _, ok := podAllocation[pod.Name]; ok {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
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

func (r *PoolReconciler) createPoolPod(ctx context.Context, pool *sandboxv1alpha1.Pool, latestRevision string) error {
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
	r.Recorder.Eventf(pool, corev1.EventTypeNormal, "SuccessfulCreate", "Created pool pod: %v", pod.Name)
	return nil
}
