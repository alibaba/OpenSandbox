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

//go:build linux

package dnsproxy

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
)

// dialerWithMark sets SO_MARK so iptables can RETURN marked packets (bypass
// redirect for proxy's own upstream DNS queries).
func (p *Proxy) dialerWithMark() *net.Dialer {
	return &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			if err := c.Control(func(fd uintptr) {
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, constants.MarkValue)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
}
