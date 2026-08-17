package platform

import "sync"

type CapacityResource struct {
	mu          sync.Mutex
	limit, used int
}

func NewCapacityResource(limit int) *CapacityResource { return &CapacityResource{limit: limit} }
func (r *CapacityResource) Acquire(quantity int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if quantity <= 0 || r.used+quantity > r.limit {
		return false
	}
	r.used += quantity
	return true
}
func (r *CapacityResource) Release(quantity int) {
	r.mu.Lock()
	r.used -= quantity
	if r.used < 0 {
		r.used = 0
	}
	r.mu.Unlock()
}
