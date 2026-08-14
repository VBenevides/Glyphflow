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
	State    string
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
	if current, ok := m.leases[resource]; ok {
		current.State = "EXPIRED"
		m.leases[resource] = current
	}
	m.fencing[resource]++
	lease := Lease{Resource: resource, Attempt: attempt, Token: token, Fencing: m.fencing[resource], Expires: now.Add(lifetime), State: "ACTIVE"}
	m.leases[resource] = lease
	return lease, nil
}

func (m *LeaseManager) Release(lease Lease, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.leases[lease.Resource]
	if !ok || current.State != "ACTIVE" || current.Attempt != lease.Attempt || current.Token != lease.Token || current.Fencing != lease.Fencing {
		return errors.New("stale lease release")
	}
	current.State = "RELEASED"
	m.leases[lease.Resource] = current
	_ = now
	return nil
}

func (m *LeaseManager) Renew(lease Lease, now time.Time, lifetime time.Duration) (Lease, error) {
	if lifetime <= 0 {
		return Lease{}, errors.New("lease lifetime is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.leases[lease.Resource]
	if !ok || current != lease || current.State != "ACTIVE" || !current.Expires.After(now) {
		return Lease{}, errors.New("stale lease renewal")
	}
	current.Expires = now.Add(lifetime)
	m.leases[lease.Resource] = current
	return current, nil
}
func (m *LeaseManager) Expire(now time.Time) []Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	var expired []Lease
	for resource, lease := range m.leases {
		if lease.State == "ACTIVE" && !lease.Expires.After(now) {
			lease.State = "EXPIRED"
			m.leases[resource] = lease
			expired = append(expired, lease)
		}
	}
	return expired
}
