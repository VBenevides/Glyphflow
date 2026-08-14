package platform

import (
	"errors"
	"sync"
)

var (
	ErrLastAdministrator   = errors.New("cannot remove or disable the last active administrator")
	ErrSystemAdministrator = errors.New("system administrator cannot be changed")
	ErrSystemRole          = errors.New("system roles cannot be changed")
)

// AdministratorGuard serializes destructive administrator changes. The
// database adapter can apply the same invariant while holding its transaction
// coordination row lock.
type AdministratorGuard struct {
	mu     sync.Mutex
	active map[string]bool
}

func NewAdministratorGuard(administrators ...string) *AdministratorGuard {
	active := make(map[string]bool, len(administrators))
	for _, administrator := range administrators {
		if administrator != "" {
			active[administrator] = true
		}
	}
	return &AdministratorGuard{active: active}
}

func (g *AdministratorGuard) Remove(id string, mutate func() error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.active[id] {
		return errors.New("administrator is not active")
	}
	if len(g.active) <= 1 {
		return ErrLastAdministrator
	}
	if mutate != nil {
		if err := mutate(); err != nil {
			return err
		}
	}
	delete(g.active, id)
	return nil
}

func (g *AdministratorGuard) Add(id string) {
	if id == "" {
		return
	}
	g.mu.Lock()
	g.active[id] = true
	g.mu.Unlock()
}

func (g *AdministratorGuard) Set(administrators ...string) {
	active := make(map[string]bool, len(administrators))
	for _, administrator := range administrators {
		if administrator != "" {
			active[administrator] = true
		}
	}
	g.mu.Lock()
	g.active = active
	g.mu.Unlock()
}

func ValidateRoleMutation(systemRole bool) error {
	if systemRole {
		return ErrSystemRole
	}
	return nil
}

func (g *AdministratorGuard) ActiveCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.active)
}
