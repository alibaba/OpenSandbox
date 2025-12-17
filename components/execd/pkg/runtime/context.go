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

package runtime

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/beego/beego/v2/core/logs"
	"github.com/google/uuid"
	"k8s.io/client-go/util/retry"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter"
	jupytersession "github.com/alibaba/opensandbox/execd/pkg/jupyter/session"
)

// CreateContext provisions a kernel-backed session and returns its ID.
func (c *Controller) CreateContext(req *CreateContextRequest) (string, error) {
	var (
		client  *jupyter.Client
		session *jupytersession.Session
		err     error
	)

	err = retry.OnError(kernelWaitingBackoff, func(err error) bool {
		logs.Error("failed to create session, retrying: %v", err)
		return err != nil
	}, func() error {
		client, session, err = c.createContext(*req)
		return err
	})
	if err != nil {
		return "", err
	}

	kernel := &jupyterKernel{
		kernelID: session.Kernel.ID,
		client:   client,
	}
	c.storeJupyterKernel(session.ID, kernel)

	err = c.setWorkingDir(kernel, req)
	if err != nil {
		return "", fmt.Errorf("failed to setup working dir: %w", err)
	}

	return session.ID, nil
}

func (c *Controller) newContextID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

func (c *Controller) newIpynbPath(sessionID, cwd string) (string, error) {
	if cwd != "" {
		err := os.MkdirAll(cwd, os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(cwd, fmt.Sprintf("%s.ipynb", sessionID)), nil
}

// createDefaultLanguageContext prewarms a session for stateless execution.
func (c *Controller) createDefaultLanguageContext(language Language) error {
	var (
		client  *jupyter.Client
		session *jupytersession.Session
		err     error
	)
	err = retry.OnError(kernelWaitingBackoff, func(err error) bool {
		logs.Error("failed to create context, retrying: %v", err)
		return err != nil
	}, func() error {
		client, session, err = c.createContext(CreateContextRequest{
			Language: language,
			Cwd:      "",
		})
		return err
	})
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.defaultLanguageJupyterSessions[language] = session.ID
	c.jupyterClientMap[session.ID] = &jupyterKernel{
		kernelID: session.Kernel.ID,
		client:   client,
	}
	return nil
}

// createContext performs the actual context creation workflow.
func (c *Controller) createContext(request CreateContextRequest) (*jupyter.Client, *jupytersession.Session, error) {
	httpClient := &http.Client{
		Transport: &jupyter.AuthTransport{
			Token: c.token,
			Base:  http.DefaultTransport,
		},
	}

	client := jupyter.NewClient(c.baseURL,
		jupyter.WithToken(c.token),
		jupyter.WithHTTPClient(httpClient))

	kernel, err := c.searchKernel(client, request.Language)
	if err != nil {
		return nil, nil, err
	}

	sessionID := c.newContextID()
	ipynb, err := c.newIpynbPath(sessionID, request.Cwd)
	if err != nil {
		return nil, nil, err
	}

	jupyterSession, err := client.CreateSession(sessionID, ipynb, kernel)
	if err != nil {
		return nil, nil, err
	}

	kernels, err := client.ListKernels()
	if err != nil {
		return nil, nil, err
	}

	found := false
	for _, k := range kernels {
		if k.ID == jupyterSession.Kernel.ID {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, errors.New("kernel not found")
	}

	return client, jupyterSession, nil
}

// storeJupyterKernel caches a session -> kernel mapping.
func (c *Controller) storeJupyterKernel(sessionID string, kernel *jupyterKernel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.jupyterClientMap[sessionID] = kernel
}
