package platform

import (
	"testing"
	"time"
)

func TestRateLimiterBoundsAuthenticationAttempts(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	now := time.Now()
	if !limiter.Allow("user|address", now) || !limiter.Allow("user|address", now) {
		t.Fatal("allowed attempts were rejected")
	}
	if limiter.Allow("user|address", now) {
		t.Fatal("third attempt was allowed")
	}
	if !limiter.Allow("other-user|address", now) {
		t.Fatal("rate limit leaked between keys")
	}
}
