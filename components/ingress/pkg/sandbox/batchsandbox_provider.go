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

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	clientset "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/client/clientset/versioned"
	informers "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/client/informers/externalversions"
	listers "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/client/listers/sandbox/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/utils"
)

// BatchSandboxProvider implements Provider interface for BatchSandbox resources
type BatchSandboxProvider struct {
	informerFactory informers.SharedInformerFactory
	lister          listers.BatchSandboxLister
	informerSynced  cache.InformerSynced
	namespace       string
}

// NewBatchSandboxProvider creates a new BatchSandboxProvider
func NewBatchSandboxProvider(
	config *rest.Config,
	namespace string,
	resyncPeriod time.Duration,
) *BatchSandboxProvider {
	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create sandbox clientset: %v", err))
	}

	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		resyncPeriod,
		informers.WithNamespace(namespace),
	)

	batchSandboxInformer := informerFactory.Sandbox().V1alpha1().BatchSandboxes()

	return &BatchSandboxProvider{
		informerFactory: informerFactory,
		lister:          batchSandboxInformer.Lister(),
		informerSynced:  batchSandboxInformer.Informer().HasSynced,
		namespace:       namespace,
	}
}

// Start starts the informer factory and waits for cache sync
func (p *BatchSandboxProvider) Start(ctx context.Context) error {
	p.informerFactory.Start(ctx.Done())

	// Wait for cache sync
	if !cache.WaitForCacheSync(ctx.Done(), p.informerSynced) {
		return errors.New("failed to sync BatchSandbox informer cache")
	}

	return nil
}

// GetEndpoint retrieves the endpoint IP for a BatchSandbox
func (p *BatchSandboxProvider) GetEndpoint(name string) (string, error) {
	// Get BatchSandbox from cache using lister with provider's namespace
	batchSandbox, err := p.lister.BatchSandboxes(p.namespace).Get(name)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s/%s", ErrSandboxNotFound, p.namespace, name)
		}
		return "", fmt.Errorf("failed to get BatchSandbox %s/%s: %w", p.namespace, name, err)
	}

	// Check if BatchSandbox is ready
	if batchSandbox.Status.Ready < 1 {
		return "", fmt.Errorf("%w: %s/%s (ready: %d/%d)",
			ErrSandboxNotReady, p.namespace, name, batchSandbox.Status.Ready, batchSandbox.Status.Replicas)
	}

	// Get endpoints from BatchSandbox using kubernetes utils
	endpoints, err := utils.GetEndpoints(batchSandbox)
	if err != nil {
		return "", fmt.Errorf("%w: %s/%s: %w", ErrSandboxNotReady, p.namespace, name, err)
	}

	// Return the first available endpoint
	return endpoints[0], nil
}
