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
	for _, existing := range r.versions[parent] {
		if existing == version {
			r.current[parent] = version
			return nil
		}
	}
	r.versions[parent] = append(r.versions[parent], version)
	r.current[parent] = version
	return nil
}

func (r *VersionRegistry) ActivateMany(activations map[string]string) error {
	if len(activations) == 0 {
		return errors.New("activations are required")
	}
	for parent, version := range activations {
		if parent == "" || version == "" {
			return errors.New("parent and version are required")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for parent, version := range activations {
		found := false
		for _, existing := range r.versions[parent] {
			if existing == version {
				found = true
				break
			}
		}
		if !found {
			r.versions[parent] = append(r.versions[parent], version)
		}
		r.current[parent] = version
	}
	return nil
}

func (r *VersionRegistry) Current(parent string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current[parent]
}
