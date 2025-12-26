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
