// Copyright 2026 The OpenSandbox Authors
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

package opensandbox

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig controls automatic retry behavior for transient errors.
// A zero-value config disables retries.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts after the initial
	// request. 0 means no retries (only the original attempt).
	MaxRetries int

	// InitialBackoff is the delay before the first retry.
	InitialBackoff time.Duration

	// MaxBackoff caps the delay between retries. Zero means no cap.
	MaxBackoff time.Duration

	// Multiplier scales the backoff after each retry attempt. Values less
	// than or equal to zero are treated as 1 (constant backoff).
	Multiplier float64

	// Jitter adds randomness to avoid thundering herd. Expressed as a
	// fraction of the computed delay: 0.0 = no jitter, 0.25 = +/-25%.
	Jitter float64

	// RetryableStatusCodes optionally overrides which HTTP status codes are
	// treated as transient for retry decisions. When empty, SDK defaults are
	// used (429, 502, 503, 504).
	RetryableStatusCodes []int

	// WakeRetryEnabled controls automatic retry of gateway wake-timeout
	// responses (OSEP-0024): a 503 carrying the "OpenSandbox-Wake: expired"
	// marker proves the request was never forwarded, so the original request
	// is resent for any method after waiting per Retry-After. Bare 503
	// responses are never affected by this knob. A nil pointer means
	// enabled — hand-built configs keep the contract unless they opt out
	// explicitly via WakeRetryDisabled(). Wake retry requires WithRetry to
	// be configured.
	WakeRetryEnabled *bool

	// WakeMaxAttempts is the maximum number of marked wake-timeout responses
	// consumed before the wake retry gives up with a SandboxWakeTimeoutError.
	// Zero falls back to DefaultWakeMaxAttempts.
	WakeMaxAttempts int

	// WakeOverallDeadline is the wall-clock cap spanning all wake cycles of
	// one logical request: waits, retries, and request time. Wake retries
	// never reset it; a next Retry-After that does not fit the remaining
	// budget ends the wake retry with a SandboxWakeTimeoutError. Zero falls
	// back to DefaultWakeOverallDeadline; a negative value removes the time
	// bound (the attempt count still applies).
	WakeOverallDeadline time.Duration
}

// DefaultWakeMaxAttempts is the default wake-retry attempt budget.
const DefaultWakeMaxAttempts = 5

// DefaultWakeOverallDeadline is the default wall-clock cap spanning all wake
// cycles of one logical request.
const DefaultWakeOverallDeadline = 60 * time.Second

// WakeRetryDisabled returns a pointer to false, for callers opting out of
// the wake retry contract on a hand-built RetryConfig. nil (the zero-value
// pointer) means enabled.
func WakeRetryDisabled() *bool {
	f := false
	return &f
}

// wakeEnabled reports the effective wake switch for a config.
func (r *RetryConfig) wakeEnabled() bool {
	return r.WakeRetryEnabled == nil || *r.WakeRetryEnabled
}

// wakeMarkerHeader is the gateway-owned OSEP-0024 marker header. The gateway
// strips it from every upstream response, so its presence on a 503 proves
// gateway origin: the request was parked and never forwarded, which is what
// makes any-method retry safe.
const wakeMarkerHeader = "OpenSandbox-Wake"

// wakeMarkerExpired is the marked value of wakeMarkerHeader.
const wakeMarkerExpired = "expired"

// retryAfterCap is a fixed operational ceiling for server-supplied
// Retry-After waits. Not configurable: ignoring a server's back-pressure
// signal is never a safe default, and the 60s guard only defends against a
// pathological header.
const retryAfterCap = 60 * time.Second

// wakeDefaultRetryAfter is the fallback wait when a marked response arrives
// without a usable Retry-After header; the gateway emits Retry-After: 1.
const wakeDefaultRetryAfter = time.Second

// DefaultRetryConfig returns a retry configuration suitable for most SDK
// consumers: 3 retries, 500ms initial backoff, 2x multiplier, 30s cap, and
// the OSEP-0024 wake retry enabled (5 attempts / 60s budget).
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.25,
		RetryableStatusCodes: []int{
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		},
		WakeRetryEnabled:    nil, // enabled by default
		WakeMaxAttempts:     DefaultWakeMaxAttempts,
		WakeOverallDeadline: DefaultWakeOverallDeadline,
	}
}

// WithRetry enables automatic retry with exponential backoff for transient
// errors and network failures. By default, transient status codes are
// 429/502/503/504; override RetryConfig.RetryableStatusCodes to customize.
func WithRetry(cfg RetryConfig) Option {
	return func(c *Client) {
		c.retry = &cfg
	}
}

// IsTransient reports whether the API error represents a transient server
// condition that may succeed on retry.
func (e *APIError) IsTransient() bool {
	return isTransientStatus(e.StatusCode)
}

// isTransientStatus classifies HTTP status codes.
//
//	Retryable: 429 (rate limit), 502, 503, 504 (infrastructure).
//	Permanent: everything else (400, 401, 403, 404, 409, 422, ...).
func isTransientStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (r *RetryConfig) isRetryableStatus(code int) bool {
	codes := r.RetryableStatusCodes
	if len(codes) == 0 {
		return isTransientStatus(code)
	}
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

// isTransientError checks whether err should trigger a retry. It handles
// *APIError (HTTP status classification) and net.Error (network-level).
func isTransientError(err error, cfg *RetryConfig) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if cfg != nil {
			return cfg.isRetryableStatus(apiErr.StatusCode)
		}
		return apiErr.IsTransient()
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// backoff computes the delay for attempt n (0-indexed) with optional jitter.
func (r *RetryConfig) backoff(attempt int) time.Duration {
	// A zero-value Multiplier would collapse every delay to zero.
	multiplier := r.Multiplier
	if multiplier <= 0 {
		multiplier = 1
	}
	delay := float64(r.InitialBackoff) * math.Pow(multiplier, float64(attempt))
	if r.MaxBackoff > 0 && delay > float64(r.MaxBackoff) {
		delay = float64(r.MaxBackoff)
	}
	if r.Jitter > 0 {
		jitter := delay * r.Jitter
		delay = delay - jitter + rand.Float64()*2*jitter
	}
	return time.Duration(delay)
}

// retryDelay returns the backoff duration, respecting Retry-After if present.
func retryDelay(cfg *RetryConfig, attempt int, err error) time.Duration {
	computed := cfg.backoff(attempt)

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		if apiErr.RetryAfter > computed {
			return apiErr.RetryAfter
		}
	}
	return computed
}

// parseRetryAfter extracts the Retry-After header value as a duration.
// Returns 0 if the header is absent or unparseable.
func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	if d, ok := parseRetryAfterValue(resp.Header.Get("Retry-After")); ok {
		return d
	}
	return 0
}

// parseRetryAfterValue parses a raw Retry-After value (delta-seconds or
// HTTP-date). ok is true only when the header is present and parseable;
// an explicit "0" (or a past date) yields (0, true).
func parseRetryAfterValue(val string) (time.Duration, bool) {
	if val == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
		if secs < 0 {
			return 0, true
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(val); err == nil {
		if d := time.Until(t); d < 0 {
			return 0, true
		} else {
			return d, true
		}
	}
	return 0, false
}

// retrySleep waits for d or until ctx is cancelled.
func retrySleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// withRetry executes fn, retrying on transient errors per the client's
// RetryConfig. If retry is nil, or both MaxRetries and the wake contract are
// inactive, fn is called once.
//
// The OSEP-0024 wake contract is enforced on top of the generic policy: a
// marked wake-timeout error (503 + "OpenSandbox-Wake: expired") is retried
// for any method — request bodies are re-marshaled per attempt, so replay is
// always safe — under its own attempt count and wall-clock budget. Bare 503
// responses follow the generic rules untouched.
func (c *Client) withRetry(ctx context.Context, fn func() error) error {
	return c.withRetryWake(ctx, true, fn)
}

// withRetryWake is withRetry with control over the wake contract. Streaming
// connects pass wakeEnabled=false: an established stream is never re-executed,
// and a waking sandbox is not parked for up to the wake budget — the response
// falls back to the generic policy's bare-503 rules (retried only when the
// method/status qualify), matching the other official SDKs, which leave their
// streaming transports unwrapped.
func (c *Client) withRetryWake(ctx context.Context, wakeEnabled bool, fn func() error) error {
	if c.retry == nil || (c.retry.MaxRetries == 0 && !(wakeEnabled && c.retry.wakeEnabled())) {
		return fn()
	}

	wakeMaxAttempts := c.retry.WakeMaxAttempts
	if wakeMaxAttempts <= 0 {
		wakeMaxAttempts = DefaultWakeMaxAttempts
	}
	var wakeDeadline time.Duration
	switch {
	case c.retry.WakeOverallDeadline < 0:
		wakeDeadline = -1 // unbounded
	case c.retry.WakeOverallDeadline == 0:
		wakeDeadline = DefaultWakeOverallDeadline
	default:
		wakeDeadline = c.retry.WakeOverallDeadline
	}

	var lastErr error
	var wakeAttempts int
	retriesUsed := 0
	started := time.Now()
	for {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		var apiErr *APIError
		if wakeEnabled && c.retry.wakeEnabled() && errors.As(lastErr, &apiErr) && apiErr.WakeExpired {
			wakeAttempts++
			elapsed := time.Since(started)
			// An explicit Retry-After of 0 means retry immediately; only a
			// missing or unparseable header falls back to the 1s default.
			wait := wakeDefaultRetryAfter
			if apiErr.RetryAfterPresent {
				wait = apiErr.RetryAfter
			}
			if wait > retryAfterCap {
				wait = retryAfterCap
			}
			// Budget checks bound retries, not a committed resume: the
			// server-side restore is never canceled by giving up here.
			exhausted := wakeAttempts >= wakeMaxAttempts
			if !exhausted && wakeDeadline >= 0 && wait > wakeDeadline-elapsed {
				exhausted = true
			}
			if exhausted {
				return &SandboxWakeTimeoutError{
					Attempts: wakeAttempts,
					Waited:   elapsed,
					LastErr:  apiErr,
				}
			}
			if err := retrySleep(ctx, wait); err != nil {
				return err
			}
			continue
		}

		if !isTransientError(lastErr, c.retry) {
			return lastErr
		}
		if retriesUsed >= c.retry.MaxRetries {
			return lastErr
		}
		retriesUsed++
		delay := retryDelay(c.retry, retriesUsed-1, lastErr)
		if err := retrySleep(ctx, delay); err != nil {
			return err
		}
	}
}
