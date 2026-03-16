// Copyright 2026 Alibaba Group Holding Ltd.
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

//go:build windows
// +build windows

package runtime

import (
	"context"
	"errors"
)

var errBashSessionNotSupported = errors.New("bash session is not supported on windows")

// BashSessionStatus holds observable state for a bash session.
type BashSessionStatus struct {
	SessionID    string
	Running      bool
	OutputOffset int64
}

// CreateBashSession is not supported on Windows.
func (c *Controller) CreateBashSession(_ *CreateContextRequest) (string, error) { //nolint:revive
	return "", errBashSessionNotSupported
}

// RunInBashSession is not supported on Windows.
func (c *Controller) RunInBashSession(_ context.Context, _ *ExecuteCodeRequest) error { //nolint:revive
	return errBashSessionNotSupported
}

// DeleteBashSession is not supported on Windows.
func (c *Controller) DeleteBashSession(_ string) error { //nolint:revive
	return errBashSessionNotSupported
}

// GetBashSession is not supported on Windows.
func (c *Controller) GetBashSession(_ string) BashSession { //nolint:revive
	return nil
}

// GetBashSessionStatus is not supported on Windows.
func (c *Controller) GetBashSessionStatus(_ string) (*BashSessionStatus, error) { //nolint:revive
	return nil, errBashSessionNotSupported
}

// ReplaySessionOutput is not supported on Windows.
func (c *Controller) ReplaySessionOutput(_ string, _ int64) ([]byte, int64, error) { //nolint:revive
	return nil, 0, errBashSessionNotSupported
}

// WriteSessionOutput is not supported on Windows.
func (c *Controller) WriteSessionOutput(_ string, _ []byte) {} //nolint:revive
