package platform

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	items  map[string][]time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, items: make(map[string][]time.Time)}
}

func (r *RateLimiter) Allow(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := now.Add(-r.window)
	values := r.items[key][:0]
	for _, value := range r.items[key] {
		if value.After(cutoff) {
			values = append(values, value)
		}
	}
	if len(values) >= r.limit {
		r.items[key] = values
		return false
	}
	r.items[key] = append(values, now)
	return true
}
