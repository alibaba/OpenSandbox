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

package proxy

import "net/http"

var (
	XRealIP         = http.CanonicalHeaderKey("X-Real-IP")
	XForwardedFor   = http.CanonicalHeaderKey("X-Forwarded-For")
	XForwardedProto = http.CanonicalHeaderKey("X-Forwarded-Proto")

	SandboxIngress = http.CanonicalHeaderKey("OPEN-SANDBOX-INGRESS")

	AccessControlAllowOrigin  = http.CanonicalHeaderKey("Access-Control-Allow-Origin")
	ReverseProxyServerPowerBy = http.CanonicalHeaderKey("Reverse-Proxy-Server-PowerBy")

	SecWebSocketProtocol = http.CanonicalHeaderKey("Sec-WebSocket-Protocol")
	Cookie               = http.CanonicalHeaderKey("Cookie")
	SetCookie            = http.CanonicalHeaderKey("Set-Cookie")
	Host                 = http.CanonicalHeaderKey("Host")
	Origin               = http.CanonicalHeaderKey("Origin")
)
