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
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
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

	RecyclingPods sets.Set[string]
}

type AllocStatus struct {
	// PodAllocation maps pod name to sandbox name for currently allocated pods.
	PodAllocation map[string]string
	// PodSupplement is the number of additional pods needed to meet sandbox demands.
	PodSupplement int32
}

type SandboxSyncInfo struct {
	SandboxName string
	Pods        []string
	Sandbox     *sandboxv1alpha1.BatchSandbox
}

type Allocator interface {
	Schedule(ctx context.Context, spec *AllocSpec) (*AllocStatus, []SandboxSyncInfo, bool, error)
	PersistPoolAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, status *AllocStatus) error
	SyncSandboxAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox, pods []string) error
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

func (allocator *defaultAllocator) Schedule(ctx context.Context, spec *AllocSpec) (*AllocStatus, []SandboxSyncInfo, bool, error) {
	log := logf.FromContext(ctx)
	status, err := allocator.initAllocation(ctx, spec)
	if err != nil {
		return nil, nil, false, err
	}
	availablePods := make([]string, 0)
	for _, pod := range spec.Pods {
		if _, ok := status.PodAllocation[pod.Name]; ok {
			continue
		}
		if spec.RecyclingPods.Has(pod.Name) {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		availablePods = append(availablePods, pod.Name)
	}
	sandboxToPods := make(map[string][]string)
	for podName, sandboxName := range status.PodAllocation {
		sandboxToPods[sandboxName] = append(sandboxToPods[sandboxName], podName)
	}
	sandboxAlloc, dirtySandboxes, poolAllocate, err := allocator.allocate(ctx, status, sandboxToPods, availablePods, spec.Sandboxes)
	if err != nil {
		log.Error(err, "allocate failed")
	}
	poolDeallocate, err := allocator.deallocate(ctx, status, sandboxToPods, spec.Sandboxes, spec.RecyclingPods)
	if err != nil {
		log.Error(err, "deallocate failed")
	}

	poolDirty := poolDeallocate || poolAllocate

	// Build pending sync list instead of immediately syncing
	var pendingSyncs []SandboxSyncInfo
	if len(dirtySandboxes) > 0 {
		sbxMap := make(map[string]*sandboxv1alpha1.BatchSandbox)
		for _, sbx := range spec.Sandboxes {
			sbxMap[sbx.Name] = sbx
		}
		for _, name := range dirtySandboxes {
			if sbx, ok := sbxMap[name]; ok {
				pendingSyncs = append(pendingSyncs, SandboxSyncInfo{
					SandboxName: name,
					Pods:        sandboxAlloc[name],
					Sandbox:     sbx,
				})
			}
		}
	}

	return status, pendingSyncs, poolDirty, nil
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

func (allocator *defaultAllocator) allocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, availablePods []string, sandboxes []*sandboxv1alpha1.BatchSandbox) (map[string][]string, []string, bool, error) {
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
	log := logf.FromContext(ctx)
	sandboxDirty := false
	poolAllocate := false
	sandboxAlloc := make([]string, 0)
	remainAvailablePods := availablePods
	if sbx.DeletionTimestamp != nil {
		log.V(1).Info("Sandbox is being deleted, skip allocation", "sandbox", sbx.Name)
		return sandboxAlloc, remainAvailablePods, false, false, nil
	}
	sbxAlloc, err := allocator.syncer.GetAllocation(ctx, sbx)
	if err != nil {
		return nil, remainAvailablePods, false, false, err
	}
	remoteAlloc := sbxAlloc.Pods
	allocatedPod := make([]string, 0)
	allocatedPod = append(allocatedPod, remoteAlloc...)
	sbxName := sbx.Name
	if localAlloc, ok := sandboxToPods[sbxName]; ok {
		for _, localPod := range localAlloc {
			if !slices.Contains(remoteAlloc, localPod) {
				sandboxDirty = true
				allocatedPod = append(allocatedPod, localPod)
			}
		}
	}
	sandboxAlloc = append(sandboxAlloc, allocatedPod...)
	needAllocateCnt := cnt - int32(len(allocatedPod))
	canAllocateCnt := needAllocateCnt
	if int32(len(availablePods)) < canAllocateCnt {
		canAllocateCnt = int32(len(availablePods))
	}
	pods := availablePods[:canAllocateCnt]
	remainAvailablePods = availablePods[canAllocateCnt:]
	sandboxToPods[sbxName] = pods
	for _, pod := range pods {
		if existingSandbox, exists := status.PodAllocation[pod]; exists {
			if existingSandbox != sbxName {
				log.Error(nil, "Pod already allocated to different sandbox, skipping",
					"pod", pod, "currentSandbox", sbxName, "existingSandbox", existingSandbox)
				continue
			}
			sandboxDirty = true
			sandboxAlloc = append(sandboxAlloc, pod)
			continue
		}
		sandboxDirty = true
		status.PodAllocation[pod] = sbxName
		poolAllocate = true
		sandboxAlloc = append(sandboxAlloc, pod)
		log.V(1).Info("Pod allocated to sandbox", "pod", pod, "sandbox", sbxName)
	}
	if canAllocateCnt < needAllocateCnt {
		status.PodSupplement += needAllocateCnt - canAllocateCnt
		log.Info("Insufficient pods for sandbox", "sandbox", sbxName, "need", needAllocateCnt, "available", canAllocateCnt, "supplement", needAllocateCnt-canAllocateCnt)
	}
	return sandboxAlloc, remainAvailablePods, sandboxDirty, poolAllocate, nil
}

func (allocator *defaultAllocator) deallocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, sandboxes []*sandboxv1alpha1.BatchSandbox, recycling sets.Set[string]) (bool, error) {
	log := logf.FromContext(ctx)
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
	sbxGC := make([]string, 0)
	for name := range sandboxToPods {
		if _, ok := sbxMap[name]; !ok {
			sbxGC = append(sbxGC, name)
		}
	}
	for _, name := range sbxGC {
		pods := sandboxToPods[name]
		log.Info("GC deleted sandbox allocation", "sandbox", name, "podCount", len(pods))
		for _, pod := range pods {
			if recycling.Has(pod) {
				continue
			}
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
		if _, ok := status.PodAllocation[pod]; ok {
			delete(status.PodAllocation, pod)
			deallocate = true
		}
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
	log := logf.FromContext(ctx)
	alloc := &PoolAllocation{}
	alloc.PodAllocation = status.PodAllocation
	log.Info("Persisting pool allocation", "pool", pool.Name, "allocations", len(status.PodAllocation))
	return allocator.store.SetAllocation(ctx, pool, alloc)
}

func (allocator *defaultAllocator) SyncSandboxAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox, pods []string) error {
	log := logf.FromContext(ctx)
	log.Info("Syncing sandbox allocation", "sandbox", sandbox.Name, "pods", pods)
	allocation := &SandboxAllocation{Pods: pods}
	return allocator.syncer.SetAllocation(ctx, sandbox, allocation)
}
