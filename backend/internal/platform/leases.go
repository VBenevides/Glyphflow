package platform

import (
	"errors"
	"sync"
	"time"
)

type Lease struct {
	Resource string
	Attempt  string
	Token    string
	Fencing  uint64
	Expires  time.Time
}

type LeaseManager struct {
	mu      sync.Mutex
	leases  map[string]Lease
	fencing map[string]uint64
}

func NewLeaseManager() *LeaseManager {
	return &LeaseManager{leases: make(map[string]Lease), fencing: make(map[string]uint64)}
}

func (m *LeaseManager) Acquire(resource, attempt, token string, now time.Time, lifetime time.Duration) (Lease, error) {
	if resource == "" || attempt == "" || token == "" || lifetime <= 0 {
		return Lease{}, errors.New("lease inputs are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.leases[resource]; ok && current.Expires.After(now) {
		return Lease{}, errors.New("resource is already leased")
	}
	m.fencing[resource]++
	lease := Lease{Resource: resource, Attempt: attempt, Token: token, Fencing: m.fencing[resource], Expires: now.Add(lifetime)}
	m.leases[resource] = lease
	return lease, nil
}

func (m *LeaseManager) Release(lease Lease, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.leases[lease.Resource]
	if !ok || current.Attempt != lease.Attempt || current.Token != lease.Token || current.Fencing != lease.Fencing {
		return errors.New("stale lease release")
	}
	delete(m.leases, lease.Resource)
	_ = now
	return nil
}
