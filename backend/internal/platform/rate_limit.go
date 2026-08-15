package platform

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	limit       int
	maxEntries  int
	items       map[string][]time.Time
	sourceItems map[string][]time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, maxEntries: 10_000, items: make(map[string][]time.Time), sourceItems: make(map[string][]time.Time)}
}

func (r *RateLimiter) Allow(key string, now time.Time) bool {
	return r.allow(key, "", now)
}

// AllowSource applies the normal key limit and a shared source-address limit.
// Expired entries are removed on every request and the map has a fixed ceiling.
func (r *RateLimiter) AllowSource(key, source string, now time.Time) bool {
	return r.allow(key, source, now)
}

func (r *RateLimiter) allow(key, source string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := now.Add(-r.window)
	values := active(r.items[key], cutoff)
	if source != "" {
		sourceValues := active(r.sourceItems[source], cutoff)
		if len(sourceValues) >= r.limit*10 {
			r.sourceItems[source] = sourceValues
			r.items[key] = values
			return false
		}
		if _, exists := r.sourceItems[source]; !exists && len(r.sourceItems) >= r.maxEntries {
			for oldest := range r.sourceItems {
				delete(r.sourceItems, oldest)
				break
			}
		}
		r.sourceItems[source] = append(sourceValues, now)
	}
	if len(values) >= r.limit {
		r.items[key] = values
		return false
	}
	if _, exists := r.items[key]; !exists && len(r.items) >= r.maxEntries {
		for oldest := range r.items {
			delete(r.items, oldest)
			break
		}
	}
	r.items[key] = append(values, now)
	return true
}

func active(values []time.Time, cutoff time.Time) []time.Time {
	kept := values[:0]
	for _, value := range values {
		if value.After(cutoff) {
			kept = append(kept, value)
		}
	}
	return kept
}
