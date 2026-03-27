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
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{RequeueAfter: 10 * time.Second}
	if err.Error() != "scale operation rate limited" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
	if err.RequeueAfter != 10*time.Second {
		t.Errorf("unexpected requeue duration: %v", err.RequeueAfter)
	}
}

func TestNewScaleLimiter_ZeroValue(t *testing.T) {
	// This tests the logic that would be in main.go's newScaleLimiter function
	limiter := newScaleLimiter(0)
	if limiter != nil {
		t.Error("expected nil limiter for zero per-minute value")
	}
}

func TestNewScaleLimiter_NegativeValue(t *testing.T) {
	limiter := newScaleLimiter(-1)
	if limiter != nil {
		t.Error("expected nil limiter for negative per-minute value")
	}
}

func TestNewScaleLimiter_PositiveValue(t *testing.T) {
	limiter := newScaleLimiter(60) // 60 per minute = 1 per second
	if limiter == nil {
		t.Fatal("expected non-nil limiter for positive per-minute value")
	}

	// With burst=1, should allow 1 token immediately
	if !limiter.AllowN(time.Now(), 1) {
		t.Error("expected AllowN(1) to succeed on fresh limiter")
	}

	// After using the token, should be rate limited
	if limiter.AllowN(time.Now(), 1) {
		t.Error("expected AllowN(1) to fail immediately after using token")
	}
}

func TestNewScaleLimiter_RateCalculation(t *testing.T) {
	// 60 per minute should give 1 per second rate
	limiter := newScaleLimiter(60)
	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}

	// Consume the initial token
	limiter.AllowN(time.Now(), 1)

	// Wait just under 1 second - should still be limited
	time.Sleep(900 * time.Millisecond)
	if limiter.AllowN(time.Now(), 1) {
		t.Error("expected rate limit after 900ms")
	}

	// Wait a bit more to exceed 1 second total - should have new token
	time.Sleep(200 * time.Millisecond)
	if !limiter.AllowN(time.Now(), 1) {
		t.Error("expected token available after >1 second")
	}
}

// newScaleLimiter is copied from main.go for testing
func newScaleLimiter(perMinute int) *rate.Limiter {
	if perMinute <= 0 {
		return nil
	}
	limiterRate := rate.Limit(perMinute) / 60.0
	return rate.NewLimiter(limiterRate, 1)
}
