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
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/api/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/utils/expectations"
)

var (
	poolResExpectations = expectations.NewResourceVersionExpectation()
)

type AllocationStore interface {
	GetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (*PoolAllocation, error)
	SetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, allocation *PoolAllocation) error
}

type annoAllocationStore struct {
	client client.Client
}

func NewAnnoAllocationStore(client client.Client) AllocationStore {
	return &annoAllocationStore{
		client: client,
	}
}

func (store *annoAllocationStore) GetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (*PoolAllocation, error) {
	alloc := &PoolAllocation{
		PodAllocation: make(map[string]string),
	}
	poolResExpectations.Observe(pool)
	anno := pool.GetAnnotations()
	if anno == nil {
		return alloc, nil
	}
	js, ok := anno[AnnoPoolAllocStatusKey]
	if !ok {
		return alloc, nil
	}
	err := json.Unmarshal([]byte(js), alloc)
	if err != nil {
		return nil, err
	}
	return alloc, nil
}

func (store *annoAllocationStore) SetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, alloc *PoolAllocation) error {
	if satisfied, unsatisfiedDuration := poolResExpectations.IsSatisfied(pool); !satisfied {
		return fmt.Errorf("pool allocation is not ready, unsatisfiedDuration:%v", unsatisfiedDuration)
	}
	js, err := json.Marshal(alloc)
	if err != nil {
		return err
	}
	old := pool.DeepCopy()
	oldGen := int64(0)
	anno := pool.GetAnnotations()
	if anno == nil {
		anno = map[string]string{}
	}
	str, ok := anno[AnnoPoolAllocGenerationKey]
	if ok {
		oldGen, err = strconv.ParseInt(str, 10, 64)
		if err != nil {
			return err
		}
	}
	gen := strconv.FormatInt(oldGen+1, 10)
	anno[AnnoPoolAllocStatusKey] = string(js)
	anno[AnnoPoolAllocGenerationKey] = gen
	pool.SetAnnotations(anno)
	patch := client.MergeFrom(old)
	if err := store.client.Patch(ctx, pool, patch); err != nil {
		return err
	}
	poolResExpectations.Expect(pool)
	return nil
}

type AllocationSyncer interface {
	SetAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox, allocation *SandboxAllocation) error
	GetAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox) (*SandboxAllocation, error)
	GetRelease(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox) (*AllocationRelease, error)
}
type annoAllocationSyncer struct {
	client client.Client
}

func NewAnnoAllocationSyncer(client client.Client) AllocationSyncer {
	return &annoAllocationSyncer{
		client: client,
	}
}

func (syncer *annoAllocationSyncer) SetAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox, allocation *SandboxAllocation) error {
	old, ok := sandbox.DeepCopyObject().(*sandboxv1alpha1.BatchSandbox)
	if !ok {
		return fmt.Errorf("invalid object")
	}
	anno := sandbox.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	js, err := json.Marshal(allocation)
	if err != nil {
		return err
	}
	anno[AnnoAllocStatusKey] = string(js)
	sandbox.SetAnnotations(anno)
	patch := client.MergeFrom(old)
	return syncer.client.Patch(ctx, sandbox, patch)
}

func (syncer *annoAllocationSyncer) GetAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox) (*SandboxAllocation, error) {
	allocation := &SandboxAllocation{
		Pods: make([]string, 0),
	}
	anno := sandbox.GetAnnotations()
	if anno == nil {
		return allocation, nil
	}
	if raw := anno[AnnoAllocStatusKey]; raw != "" {
		err := json.Unmarshal([]byte(raw), allocation)
		if err != nil {
			return nil, err
		}
	}
	return allocation, nil
}

func (syncer *annoAllocationSyncer) GetRelease(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox) (*AllocationRelease, error) {
	release := &AllocationRelease{
		Pods: make([]string, 0),
	}
	anno := sandbox.GetAnnotations()
	if anno == nil {
		return release, nil
	}
	if raw := anno[AnnoAllocReleaseKey]; raw != "" {
		err := json.Unmarshal([]byte(raw), release)
		if err != nil {
			return nil, err
		}
	}
	return release, nil
}

type AllocSpec struct {
	// sandboxes need to allocate
	Sandboxes []*sandboxv1alpha1.BatchSandbox
	// pool
	Pool *sandboxv1alpha1.Pool
	// all pods of pool
	Pods []*corev1.Pod
}

type AllocStatus struct {
	// pod allocated to sandbox
	PodAllocation map[string]string
	// pod request count
	PodSupplement int32
}

// Allocator is responsible for managing pod allocation from Pool to BatchSandboxes.
// It performs allocation calculations and persists the allocation state.
type Allocator interface {
	// Schedule computes the allocation of pods to BatchSandboxes based on the current pool state.
	// It returns:
	//   - AllocStatus: the computed allocation state (pod-to-sandbox mapping and required supplement count)
	//   - poolDirty: indicates whether the Pool's allocation state has changed and needs persistence
	//   - error: any error during the scheduling process
	// This method only performs calculation and does not modify the Pool CR directly.
	Schedule(ctx context.Context, spec *AllocSpec) (*AllocStatus, bool, error)

	// PersistPoolAllocation persists the allocation status.
	// This method should be called after Schedule() when poolDirty is true.
	PersistPoolAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, status *AllocStatus) error
}

type defaultAllocator struct {
	store  AllocationStore
	syncer AllocationSyncer
}

func NewDefaultAllocator(client client.Client) Allocator {
	return &defaultAllocator{
		store:  NewAnnoAllocationStore(client),
		syncer: NewAnnoAllocationSyncer(client),
	}
}

func (allocator *defaultAllocator) Schedule(ctx context.Context, spec *AllocSpec) (*AllocStatus, bool, error) {
	log := logf.FromContext(ctx)
	status, err := allocator.initAllocation(ctx, spec)
	if err != nil {
		return nil, false, err
	}
	availablePods := make([]string, 0)
	for _, pod := range spec.Pods {
		if _, ok := status.PodAllocation[pod.Name]; ok { // allocated
			continue
		}
		if pod.Status.Phase != corev1.PodRunning { // not running
			continue
		}
		availablePods = append(availablePods, pod.Name)
	}
	sandboxToPods := make(map[string][]string)
	for podName, sandboxName := range status.PodAllocation {
		sandboxToPods[sandboxName] = append(sandboxToPods[sandboxName], podName)
	}
	sandboxAlloc, dirtySandboxes, poolAllocate, err := allocator.allocate(ctx, status, sandboxToPods, availablePods, spec.Sandboxes, spec.Pods)
	if err != nil {
		log.Error(err, "allocate failed")
	}
	poolDeallocate, err := allocator.deallocate(ctx, status, sandboxToPods, spec.Sandboxes)
	if err != nil {
		log.Error(err, "deallocate failed")
	}

	poolDirty := poolDeallocate || poolAllocate

	if err := allocator.syncAllocResult(ctx, dirtySandboxes, sandboxAlloc, spec.Sandboxes); err != nil {
		log.Error(err, "sync alloc result failed")
	}
	return status, poolDirty, nil // Do not return the error of sandboxes witch will block pool schedule.
}

func (allocator *defaultAllocator) initAllocation(ctx context.Context, spec *AllocSpec) (*AllocStatus, error) {
	var err error
	status := &AllocStatus{
		PodAllocation: make(map[string]string),
	}
	status.PodAllocation, err = allocator.getPodAllocation(ctx, spec.Pool)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (allocator *defaultAllocator) allocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, availablePods []string, sandboxes []*sandboxv1alpha1.BatchSandbox, pods []*corev1.Pod) (map[string][]string, []string, bool, error) {
	errs := make([]error, 0)
	sandboxAlloc := make(map[string][]string)
	dirtySandboxes := make([]string, 0)
	poolDirty := false
	for _, sbx := range sandboxes {
		alloc, remainAvailablePods, sandboxDirty, poolAllocate, err := allocator.doAllocate(ctx, status, sandboxToPods, availablePods, sbx, *sbx.Spec.Replicas)
		availablePods = remainAvailablePods
		if err != nil {
			errs = append(errs, err)
		} else {
			sandboxAlloc[sbx.Name] = alloc
			if sandboxDirty {
				dirtySandboxes = append(dirtySandboxes, sbx.Name)
			}
			if poolAllocate {
				poolDirty = true
			}
		}
	}
	return sandboxAlloc, dirtySandboxes, poolDirty, gerrors.Join(errs...)
}

func (allocator *defaultAllocator) doAllocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, availablePods []string, sbx *sandboxv1alpha1.BatchSandbox, cnt int32) ([]string, []string, bool, bool, error) {
	sandboxDirty := false
	poolAllocate := false
	sandboxAlloc := make([]string, 0)
	remainAvailablePods := availablePods
	if sbx.DeletionTimestamp != nil {
		return sandboxAlloc, remainAvailablePods, false, false, nil
	}
	sbxAlloc, err := allocator.syncer.GetAllocation(ctx, sbx)
	if err != nil {
		return nil, remainAvailablePods, false, false, err
	}
	remoteAlloc := sbxAlloc.Pods
	allocatedPod := make([]string, 0)
	allocatedPod = append(allocatedPod, remoteAlloc...)
	name := sbx.Name
	if localAlloc, ok := sandboxToPods[name]; ok {
		for _, localPod := range localAlloc {
			if !slices.Contains(remoteAlloc, localPod) {
				sandboxDirty = true
				allocatedPod = append(allocatedPod, localPod)
			}
		}
	}
	sandboxAlloc = append(sandboxAlloc, allocatedPod...) // old allocation
	needAllocateCnt := cnt - int32(len(allocatedPod))
	canAllocateCnt := needAllocateCnt
	if int32(len(availablePods)) < canAllocateCnt {
		canAllocateCnt = int32(len(availablePods))
	}
	pods := availablePods[:canAllocateCnt]
	remainAvailablePods = availablePods[canAllocateCnt:]
	sandboxToPods[name] = pods
	for _, pod := range pods {
		sandboxDirty = true
		status.PodAllocation[pod] = name
		poolAllocate = true
		sandboxAlloc = append(sandboxAlloc, pod) // new allocation
	}
	if canAllocateCnt < needAllocateCnt {
		status.PodSupplement += needAllocateCnt - canAllocateCnt
	}
	return sandboxAlloc, remainAvailablePods, sandboxDirty, poolAllocate, nil
}

func (allocator *defaultAllocator) deallocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, sandboxes []*sandboxv1alpha1.BatchSandbox) (bool, error) {
	poolDeallocate := false
	errs := make([]error, 0)
	sbxMap := make(map[string]*sandboxv1alpha1.BatchSandbox)
	for _, sandbox := range sandboxes {
		sbxMap[sandbox.Name] = sandbox
		deallocate, err := allocator.doDeallocate(ctx, status, sandboxToPods, sandbox)
		if err != nil {
			errs = append(errs, err)
		} else {
			if deallocate {
				poolDeallocate = true
			}
		}
	}
	// gc deleted sandbox and  batch sandbox
	SandboxGC := make([]string, 0)
	for name := range sandboxToPods {
		if _, ok := sbxMap[name]; !ok {
			SandboxGC = append(SandboxGC, name)
		}
	}
	for _, name := range SandboxGC {
		pods := sandboxToPods[name]
		for _, pod := range pods {
			delete(status.PodAllocation, pod)
			poolDeallocate = true
		}
		delete(sandboxToPods, name)
	}
	return poolDeallocate, gerrors.Join(errs...)
}

func (allocator *defaultAllocator) doDeallocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, sbx *sandboxv1alpha1.BatchSandbox) (bool, error) {
	deallocate := false
	name := sbx.Name
	allocatedPods, ok := sandboxToPods[name]
	if !ok { // pods is already release to pool
		return false, nil
	}
	toRelease, err := allocator.syncer.GetRelease(ctx, sbx)
	if err != nil {
		return false, err
	}
	for _, pod := range toRelease.Pods {
		delete(status.PodAllocation, pod)
		deallocate = true
	}
	pods := make([]string, 0)
	for _, pod := range allocatedPods {
		if slices.Contains(toRelease.Pods, pod) {
			continue
		}
		pods = append(pods, pod)
	}
	sandboxToPods[name] = pods
	return deallocate, nil
}

func (allocator *defaultAllocator) getPodAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (map[string]string, error) {
	alloc, err := allocator.store.GetAllocation(ctx, pool)
	if err != nil {
		return nil, err
	}
	if alloc == nil {
		return map[string]string{}, nil
	}
	return alloc.PodAllocation, nil
}

func (allocator *defaultAllocator) PersistPoolAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, status *AllocStatus) error {
	alloc := &PoolAllocation{}
	alloc.PodAllocation = status.PodAllocation
	return allocator.store.SetAllocation(ctx, pool, alloc)
}

func (allocator *defaultAllocator) syncAllocResult(ctx context.Context, dirtySandboxes []string, sandboxAlloc map[string][]string, sandboxes []*sandboxv1alpha1.BatchSandbox) error {
	if len(dirtySandboxes) == 0 {
		return nil
	}
	errs := make([]error, 0)
	sbxMap := make(map[string]*sandboxv1alpha1.BatchSandbox)
	for _, sbx := range sandboxes {
		sbxMap[sbx.Name] = sbx
	}
	for _, name := range dirtySandboxes {
		err := allocator.doSyncAllocResult(ctx, sandboxAlloc[name], sbxMap[name])
		if err != nil {
			errs = append(errs, err)
		}
	}
	return gerrors.Join(errs...)
}

func (allocator *defaultAllocator) doSyncAllocResult(ctx context.Context, allocatedPods []string, sbx *sandboxv1alpha1.BatchSandbox) error {
	allocation := &SandboxAllocation{}
	allocation.Pods = allocatedPods
	return allocator.syncer.SetAllocation(ctx, sbx, allocation)
}
