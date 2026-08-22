package controlplane

import (
	"fmt"
	"sync"
)

type Health struct {
	mu         sync.RWMutex
	components []string
	states     map[string]error
}

func NewHealth(components ...string) *Health {
	return &Health{components: append([]string(nil), components...), states: make(map[string]error, len(components))}
}

func (h *Health) MarkHealthy(component string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.states[component] = nil
}

func (h *Health) MarkFailed(component string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.states[component] = err
}

func (h *Health) Ready() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, component := range h.components {
		err, ok := h.states[component]
		if !ok {
			return fmt.Errorf("component %s has not started", component)
		}
		if err != nil {
			return fmt.Errorf("component %s is unhealthy: %w", component, err)
		}
	}
	return nil
}
