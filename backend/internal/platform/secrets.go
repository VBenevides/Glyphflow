package platform

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type SecretReference struct {
	Reference string
	Version   int
	ExpiresAt time.Time
	Revoked   bool
}

type SecretResolver struct {
	mu      sync.RWMutex
	values  map[string]string
	entries map[string]SecretReference
}

func NewSecretResolver() *SecretResolver {
	return &SecretResolver{values: make(map[string]string), entries: make(map[string]SecretReference)}
}

func (r *SecretResolver) Add(reference, value string, version int, expiresAt time.Time) error {
	if !strings.HasPrefix(reference, "secret://") || value == "" || version <= 0 {
		return errors.New("secret reference is invalid")
	}
	r.mu.Lock()
	r.values[reference] = value
	r.entries[reference] = SecretReference{Reference: reference, Version: version, ExpiresAt: expiresAt}
	r.mu.Unlock()
	return nil
}

func (r *SecretResolver) Revoke(reference string) {
	r.mu.Lock()
	entry := r.entries[reference]
	entry.Revoked = true
	r.entries[reference] = entry
	r.mu.Unlock()
}

func (r *SecretResolver) Resolve(reference string, now time.Time) (string, SecretReference, error) {
	r.mu.RLock()
	entry, ok := r.entries[reference]
	value := r.values[reference]
	r.mu.RUnlock()
	if !ok || value == "" || entry.Revoked || (!entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt)) {
		return "", SecretReference{}, errors.New("secret reference is unavailable")
	}
	return value, entry, nil
}
