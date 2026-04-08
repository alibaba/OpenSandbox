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
	"os"
	"slices"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	sandboxv1alpha1 "github.com/alibaba/OpenSandbox/sandbox-k8s/apis/sandbox/v1alpha1"
)

type AllocationStore interface {
	GetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (*PoolAllocation, error)
	SetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, allocation *PoolAllocation) error
	ClearAllocation(ctx context.Context, ns string, poolName string) error
	UpdateAllocation(ctx context.Context, ns string, poolName string, sandboxName string, pods []string)
	Recover(ctx context.Context, c client.Client) error
}

// poolEntry represents a single pool's allocation data with its own lock for fine-grained concurrency control
type poolEntry struct {
	mu   sync.RWMutex
	data map[string]string // podName -> sandboxName
}

// InMemoryAllocationStore depends on annoAllocationSyncer to get allocation info from BatchSandbox.
type InMemoryAllocationStore struct {
	poolsMu sync.RWMutex
	pools   map[string]*poolEntry
	syncer  *annoAllocationSyncer
}

func NewInMemoryAllocationStore() AllocationStore {
	return &InMemoryAllocationStore{
		pools:  make(map[string]*poolEntry),
		syncer: &annoAllocationSyncer{},
	}
}

// Recover builds the allocation map from all BatchSandboxes
// This should be called once during controller initialization before reconcile starts
func (store *InMemoryAllocationStore) Recover(ctx context.Context, c client.Client) error {
	log := logf.FromContext(ctx)
	log.Info("Starting allocation recovery from BatchSandboxes")

	batchSandboxList := &sandboxv1alpha1.BatchSandboxList{}
	if err := c.List(ctx, batchSandboxList); err != nil {
		return fmt.Errorf("failed to list batch sandboxes for recovery: %w", err)
	}

	// Build new pools map first without holding the lock
	newPools := make(map[string]*poolEntry)

	for _, sbx := range batchSandboxList.Items {
		poolRef := sbx.Spec.PoolRef
		if poolRef == "" {
			continue
		}
		allocation, err := store.syncer.GetAllocation(ctx, &sbx)
		if err != nil {
			log.Error(err, "Failed to unmarshal sandbox allocation during recovery", "sandbox", sbx.Name)
			return err
		}
		key := store.poolKey(sbx.Namespace, poolRef)
		entry, exists := newPools[key]
		if !exists {
			entry = &poolEntry{
				data: make(map[string]string),
			}
			newPools[key] = entry
		}

		for _, podName := range allocation.Pods {
			entry.data[podName] = sbx.Name
		}
		// Filter released pods
		allocRelease, err := store.syncer.GetRelease(ctx, &sbx)
		if err != nil {
			log.Error(err, "Failed to unmarshal sandbox release during recovery", "sandbox", sbx.Name)
			return err
		}
		for _, podName := range allocRelease.Pods {
			if entry.data[podName] == sbx.Name {
				delete(entry.data, podName)
			}
		}

		log.Info("Recovered sandbox allocation", "pool", poolRef, "sandbox", sbx.Name, "pods", len(allocation.Pods))
	}

	store.poolsMu.Lock()
	store.pools = newPools
	store.poolsMu.Unlock()

	log.Info("Allocation recovery completed", "totalPools", len(store.pools))
	return nil
}

func (store *InMemoryAllocationStore) ClearAllocation(ctx context.Context, ns string, poolName string) error {
	log := logf.FromContext(ctx)
	store.poolsMu.Lock()
	log.Info("Clearing pool allocation", "namespace", ns, "pool", poolName)
	delete(store.pools, store.poolKey(ns, poolName))
	store.poolsMu.Unlock()
	return nil
}

func (store *InMemoryAllocationStore) GetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (*PoolAllocation, error) {
	store.poolsMu.RLock()
	entry, exists := store.pools[store.poolKey(pool.Namespace, pool.Name)]
	store.poolsMu.RUnlock()

	alloc := &PoolAllocation{
		PodAllocation: make(map[string]string),
	}

	if !exists {
		return alloc, nil
	}

	entry.mu.RLock()
	defer entry.mu.RUnlock()

	for podName, sandboxName := range entry.data {
		alloc.PodAllocation[podName] = sandboxName
	}

	return alloc, nil
}

func (store *InMemoryAllocationStore) SetAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, alloc *PoolAllocation) error {
	entry := store.getOrCreatePool(pool.Namespace, pool.Name)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.data = make(map[string]string)
	for podName, sandboxName := range alloc.PodAllocation {
		entry.data[podName] = sandboxName
	}

	return nil
}

func (store *InMemoryAllocationStore) UpdateAllocation(ctx context.Context, ns string, poolName string, sandboxName string, pods []string) {
	entry := store.getOrCreatePool(ns, poolName)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	for podName, sbxName := range entry.data {
		if sbxName == sandboxName {
			delete(entry.data, podName)
		}
	}

	for _, podName := range pods {
		entry.data[podName] = sandboxName
	}
}

// getOrCreatePool returns the pool entry for the given pool name, creating it if necessary.
// This method uses a double-checked locking pattern to ensure thread-safe creation.
func (store *InMemoryAllocationStore) getOrCreatePool(ns string, poolName string) *poolEntry {
	store.poolsMu.RLock()
	entry, exists := store.pools[store.poolKey(ns, poolName)]
	store.poolsMu.RUnlock()

	if exists {
		return entry
	}

	store.poolsMu.Lock()
	defer store.poolsMu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists := store.pools[store.poolKey(ns, poolName)]; exists {
		return entry
	}

	entry = &poolEntry{
		data: make(map[string]string),
	}
	store.pools[store.poolKey(ns, poolName)] = entry
	return entry
}

func (store *InMemoryAllocationStore) poolKey(ns, name string) string {
	return ns + "/" + name
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
	Pool      *sandboxv1alpha1.Pool
	// all candidate pods
	Pods []*corev1.Pod
}

type AllocStatus struct {
	// pod allocated to sandbox
	PodAllocation map[string]string
	// pod request count
	PodSupplement int32
	// DirtyPods are pods that should be recycled (deleted) after release
	DirtyPods []string
}

type SandboxSyncInfo struct {
	SandboxName string
	Pods        []string
	Sandbox     *sandboxv1alpha1.BatchSandbox
}

type Allocator interface {
	Schedule(ctx context.Context, spec *AllocSpec) (*AllocStatus, []SandboxSyncInfo, bool, error)
	GetPoolAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (map[string]string, error)
	PersistPoolAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool, status *AllocStatus) error
	ClearPoolAllocation(ctx context.Context, ns string, poolName string) error
	SyncSandboxAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox, pods []string) error
}

type defaultAllocator struct {
	store       AllocationStore
	syncer      AllocationSyncer
	client      client.Client
	recoverOnce sync.Once
}

func NewDefaultAllocator(client client.Client) Allocator {
	return &defaultAllocator{
		store:  NewInMemoryAllocationStore(),
		syncer: NewAnnoAllocationSyncer(client),
		client: client,
	}
}

func (allocator *defaultAllocator) checkRecovery(ctx context.Context) {
	log := logf.FromContext(ctx)
	allocator.recoverOnce.Do(func() {
		if err := allocator.store.Recover(ctx, allocator.client); err != nil {
			log.Error(err, "Allocator state recovery failed")
			os.Exit(1)
		}
	})
}

func (allocator *defaultAllocator) Schedule(ctx context.Context, spec *AllocSpec) (*AllocStatus, []SandboxSyncInfo, bool, error) {
	log := logf.FromContext(ctx)
	log.Info("Schedule started", "pool", spec.Pool.Name, "totalPods", len(spec.Pods), "sandboxes", len(spec.Sandboxes))
	allocator.checkRecovery(ctx)
	status, err := allocator.initAllocation(ctx, spec)
	if err != nil {
		return nil, nil, false, err
	}
	availablePods := make([]string, 0)
	for _, pod := range spec.Pods {
		if _, ok := status.PodAllocation[pod.Name]; ok {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		availablePods = append(availablePods, pod.Name)
	}
	log.V(1).Info("Schedule init", "existingAllocations", len(status.PodAllocation), "availablePods", len(availablePods))
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
	status.PodAllocation, err = allocator.GetPoolAllocation(ctx, spec.Pool)
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

func (allocator *defaultAllocator) deallocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, sandboxes []*sandboxv1alpha1.BatchSandbox) (bool, error) {
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
			delete(status.PodAllocation, pod)
			poolDeallocate = true
			status.DirtyPods = append(status.DirtyPods, pod)
		}
		delete(sandboxToPods, name)
	}
	return poolDeallocate, gerrors.Join(errs...)
}

func (allocator *defaultAllocator) doDeallocate(ctx context.Context, status *AllocStatus, sandboxToPods map[string][]string, sbx *sandboxv1alpha1.BatchSandbox) (bool, error) {
	log := logf.FromContext(ctx)
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
		status.DirtyPods = append(status.DirtyPods, pod)
		log.V(1).Info("Pod released from sandbox", "pod", pod, "sandbox", name)
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

func (allocator *defaultAllocator) GetPoolAllocation(ctx context.Context, pool *sandboxv1alpha1.Pool) (map[string]string, error) {
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

func (allocator *defaultAllocator) ClearPoolAllocation(ctx context.Context, ns string, poolName string) error {
	return allocator.store.ClearAllocation(ctx, ns, poolName)
}

func (allocator *defaultAllocator) SyncSandboxAllocation(ctx context.Context, sandbox *sandboxv1alpha1.BatchSandbox, pods []string) error {
	log := logf.FromContext(ctx)
	log.Info("Syncing sandbox allocation", "sandbox", sandbox.Name, "pods", pods)
	oldState, err := allocator.syncer.GetAllocation(ctx, sandbox)
	if err != nil { // NOTE: if failed, the allocation in memory will be inconsistent with the real state in sandbox.
		return fmt.Errorf("failed to get current sandbox allocation: %w", err)
	}
	poolRef := sandbox.Spec.PoolRef
	allocation := &SandboxAllocation{Pods: pods}
	if err := allocator.syncer.SetAllocation(ctx, sandbox, allocation); err != nil {
		log.Error(err, "Rollback sandbox allocation", "sandbox", sandbox.Name, "pods", oldState.Pods)
		allocator.store.UpdateAllocation(ctx, sandbox.Namespace, poolRef, sandbox.Name, oldState.Pods)
		return err
	}
	return nil
}
