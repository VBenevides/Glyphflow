package platform

import (
	"testing"
	"time"
)

func TestRateLimiterBoundsAuthenticationAttempts(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	now := time.Now()
	first := limiter.Allow("user|address", now)
	second := limiter.Allow("user|address", now)
	if !first || !second {
		t.Fatal("allowed attempts were rejected")
	}
	if limiter.Allow("user|address", now) {
		t.Fatal("third attempt was allowed")
	}
	if !limiter.Allow("other-user|address", now) {
		t.Fatal("rate limit leaked between keys")
	}
}

func TestRateLimiterBoundsSourceAndState(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	now := time.Now()
	for i := 0; i < 10; i++ {
		if !limiter.AllowSource("user-"+string(rune('a'+i)), "198.51.100.1", now) {
			t.Fatalf("source attempt %d was rejected too early", i)
		}
	}
	if limiter.AllowSource("user-k", "198.51.100.1", now) {
		t.Fatal("source limit was not enforced")
	}
	if !limiter.AllowSource("new", "198.51.100.2", now.Add(2*time.Minute)) {
		t.Fatal("expired source state was not removed")
	}
}
