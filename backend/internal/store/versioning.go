package store

import (
	"errors"
	"sync"
)

type VersionRegistry struct {
	mu       sync.Mutex
	current  map[string]string
	versions map[string][]string
}

func NewVersionRegistry() *VersionRegistry {
	return &VersionRegistry{current: make(map[string]string), versions: make(map[string][]string)}
}

func (r *VersionRegistry) Activate(parent, version string) error {
	if parent == "" || version == "" {
		return errors.New("parent and version are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions[parent] = append(r.versions[parent], version)
	r.current[parent] = version
	return nil
}

func (r *VersionRegistry) Current(parent string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current[parent]
}
