package opensandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Transient error classification
// ---------------------------------------------------------------------------

func TestIsTransient(t *testing.T) {
	tests := []struct {
		status    int
		transient bool
	}{
		{http.StatusTooManyRequests, true},      // 429
		{http.StatusBadGateway, true},           // 502
		{http.StatusServiceUnavailable, true},   // 503
		{http.StatusGatewayTimeout, true},       // 504
		{http.StatusBadRequest, false},          // 400
		{http.StatusUnauthorized, false},        // 401
		{http.StatusForbidden, false},           // 403
		{http.StatusNotFound, false},            // 404
		{http.StatusConflict, false},            // 409
		{http.StatusUnprocessableEntity, false}, // 422
		{http.StatusInternalServerError, false}, // 500
	}

	for _, tt := range tests {
		apiErr := &APIError{StatusCode: tt.status}
		if got := apiErr.IsTransient(); got != tt.transient {
			t.Errorf("status %d: IsTransient() = %v, want %v", tt.status, got, tt.transient)
		}
	}
}

// ---------------------------------------------------------------------------
// Retry on transient errors
// ---------------------------------------------------------------------------

func TestRetry_TransientThenSuccess(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"code":"UNAVAILABLE","message":"try again"}`))
			return
		}
		jsonResponse(w, http.StatusOK, SandboxInfo{ID: "sbx-ok", CreatedAt: time.Now()})
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key", WithRetry(RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}))

	got, err := client.GetSandbox(context.Background(), "sbx-ok")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if got.ID != "sbx-ok" {
		t.Errorf("ID = %q, want %q", got.ID, "sbx-ok")
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// No retry on permanent errors
// ---------------------------------------------------------------------------

func TestRetry_PermanentError(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		jsonResponse(w, http.StatusNotFound, ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "sandbox not found",
		})
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key", WithRetry(DefaultRetryConfig()))

	_, err := client.GetSandbox(context.Background(), "sbx-missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 404)", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// Retry exhaustion
// ---------------------------------------------------------------------------

func TestRetry_Exhausted(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"UNAVAILABLE","message":"overloaded"}`))
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key", WithRetry(RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2.0,
	}))

	_, err := client.GetSandbox(context.Background(), "sbx-fail")
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", apiErr.StatusCode)
	}
	// 1 initial + 2 retries = 3
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// Context cancellation during retry
// ---------------------------------------------------------------------------

func TestRetry_ContextCancelled(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"UNAVAILABLE","message":"down"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewLifecycleClient(srv.URL, "key", WithRetry(RetryConfig{
		MaxRetries:     10,
		InitialBackoff: 30 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}))

	_, err := client.GetSandbox(ctx, "sbx-slow")
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	// Should have attempted at least once but not all 10 retries.
	if attempts.Load() < 1 {
		t.Error("expected at least 1 attempt")
	}
	if attempts.Load() > 5 {
		t.Errorf("too many attempts (%d) — context should have cancelled", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// No retry when RetryConfig is nil
// ---------------------------------------------------------------------------

func TestRetry_Disabled(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"UNAVAILABLE","message":"down"}`))
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key") // no WithRetry

	_, err := client.GetSandbox(context.Background(), "sbx-noretry")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (retry disabled)", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// Retry-After header
// ---------------------------------------------------------------------------

func TestRetry_RetryAfterHeader(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"code":"RATE_LIMITED","message":"slow down"}`))
			return
		}
		jsonResponse(w, http.StatusOK, SandboxInfo{ID: "sbx-rate", CreatedAt: time.Now()})
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key", WithRetry(RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Multiplier:     2.0,
	}))

	start := time.Now()
	got, err := client.GetSandbox(context.Background(), "sbx-rate")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if got.ID != "sbx-rate" {
		t.Errorf("ID = %q, want %q", got.ID, "sbx-rate")
	}
	// Retry-After: 1 means 1 second. The delay should be at least ~1s.
	if elapsed < 900*time.Millisecond {
		t.Errorf("elapsed = %v, expected >= ~1s from Retry-After header", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Retry on 429 (rate limit)
// ---------------------------------------------------------------------------

func TestRetry_RateLimit429(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"code":"RATE_LIMITED","message":"too fast"}`))
			return
		}
		jsonResponse(w, http.StatusOK, SandboxInfo{ID: "sbx-429", CreatedAt: time.Now()})
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key", WithRetry(RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}))

	got, err := client.GetSandbox(context.Background(), "sbx-429")
	if err != nil {
		t.Fatalf("expected success after 429 retry, got: %v", err)
	}
	if got.ID != "sbx-429" {
		t.Errorf("ID = %q, want %q", got.ID, "sbx-429")
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// Streaming retry (connection phase only)
// ---------------------------------------------------------------------------

func TestRetry_StreamingConnection(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"code":"UNAVAILABLE","message":"try again"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event: stdout\ndata: hello\n\n"))
	}))
	defer srv.Close()

	client := NewExecdClient(srv.URL, "tok", WithRetry(RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}))

	var events []StreamEvent
	err := client.RunCommand(context.Background(), RunCommandRequest{Command: "echo hello"}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after stream retry, got: %v", err)
	}
	if len(events) != 1 || events[0].Data != "hello" {
		t.Errorf("events = %+v, want [{Event:stdout Data:hello}]", events)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// Backoff computation
// ---------------------------------------------------------------------------

func TestBackoff(t *testing.T) {
	cfg := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Jitter:         0, // no jitter for deterministic test
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{10, 10 * time.Second}, // capped at MaxBackoff
	}

	for _, tt := range tests {
		got := cfg.backoff(tt.attempt)
		if got != tt.expected {
			t.Errorf("backoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestBackoff_WithJitter(t *testing.T) {
	cfg := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.5,
	}

	// With 50% jitter, attempt 0 should be in [50ms, 150ms].
	for range 20 {
		got := cfg.backoff(0)
		if got < 50*time.Millisecond || got > 150*time.Millisecond {
			t.Errorf("backoff(0) with 50%% jitter = %v, expected [50ms, 150ms]", got)
		}
	}
}

// ---------------------------------------------------------------------------
// Transport / connection pooling
// ---------------------------------------------------------------------------

func TestDefaultTransport(t *testing.T) {
	tr := DefaultTransport()
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
}

func TestTransportConfig_NewTransport(t *testing.T) {
	cfg := TransportConfig{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		DialTimeout:         15 * time.Second,
		KeepAlive:           15 * time.Second,
	}
	tr := cfg.NewTransport()
	if tr.MaxIdleConns != 50 {
		t.Errorf("MaxIdleConns = %d, want 50", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 5 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 5", tr.MaxIdleConnsPerHost)
	}
}

// ---------------------------------------------------------------------------
// ConnectionConfig with Retry and Transport
// ---------------------------------------------------------------------------

func TestConnectionConfig_RetryAndTransport(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"code":"BAD_GATEWAY","message":"retry"}`))
			return
		}
		jsonResponse(w, http.StatusOK, SandboxInfo{ID: "sbx-cfg", CreatedAt: time.Now()})
	}))
	defer srv.Close()

	retry := DefaultRetryConfig()
	retry.InitialBackoff = 10 * time.Millisecond
	transport := DefaultTransportConfig()

	config := ConnectionConfig{
		Domain:    srv.Listener.Addr().String(),
		Protocol:  "http",
		APIKey:    "test-key",
		Retry:     &retry,
		Transport: &transport,
	}

	lc := config.lifecycleClient()
	got, err := lc.GetSandbox(context.Background(), "sbx-cfg")
	if err != nil {
		t.Fatalf("expected success with ConnectionConfig retry, got: %v", err)
	}
	if got.ID != "sbx-cfg" {
		t.Errorf("ID = %q, want %q", got.ID, "sbx-cfg")
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

// ---------------------------------------------------------------------------
// APIError.Error() with request ID
// ---------------------------------------------------------------------------

func TestAPIError_ErrorWithRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-abc-123")
		jsonResponse(w, http.StatusNotFound, ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "sandbox not found",
		})
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "key")
	_, err := client.GetSandbox(context.Background(), "sbx-missing")
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %q, want %q", apiErr.RequestID, "req-abc-123")
	}

	errMsg := apiErr.Error()
	if got, want := errMsg, "NOT_FOUND: sandbox not found (request_id: req-abc-123)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// parseRetryAfter
// ---------------------------------------------------------------------------

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected time.Duration
	}{
		{"seconds", "5", 5 * time.Second},
		{"zero", "0", 0},
		{"empty", "", 0},
		{"negative", "-1", 0},
		{"garbage", "not-a-number", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}
			got := parseRetryAfter(resp)
			if got != tt.expected {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.header, got, tt.expected)
			}
		})
	}
}

func TestParseRetryAfter_NilResponse(t *testing.T) {
	got := parseRetryAfter(nil)
	if got != 0 {
		t.Errorf("parseRetryAfter(nil) = %v, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// isTransientError with wrapped errors
// ---------------------------------------------------------------------------

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"nil", nil, false},
		{"api 503", &APIError{StatusCode: 503}, true},
		{"api 429", &APIError{StatusCode: 429}, true},
		{"api 404", &APIError{StatusCode: 404}, false},
		{"api 400", &APIError{StatusCode: 400}, false},
		{"api 502", &APIError{StatusCode: 502}, true},
		{"api 504", &APIError{StatusCode: 504}, true},
		{"api 500", &APIError{StatusCode: 500}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientError(tt.err); got != tt.transient {
				t.Errorf("isTransientError(%v) = %v, want %v", tt.err, got, tt.transient)
			}
		})
	}
}

// suppress unused import warning
var _ = json.Marshal
