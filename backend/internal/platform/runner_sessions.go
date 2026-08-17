package platform

import (
	"errors"
	"sync"
)

type RunnerSessionRegistry struct {
	mu     sync.RWMutex
	active map[string]string
}

func NewRunnerSessionRegistry() *RunnerSessionRegistry {
	return &RunnerSessionRegistry{active: make(map[string]string)}
}

func (r *RunnerSessionRegistry) Connect(runnerID, sessionID string) (string, error) {
	if runnerID == "" || sessionID == "" {
		return "", errors.New("runner and session IDs are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.active[runnerID]
	r.active[runnerID] = sessionID
	return previous, nil
}

func (r *RunnerSessionRegistry) IsActive(runnerID, sessionID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active[runnerID] == sessionID
}

func (r *RunnerSessionRegistry) Disconnect(runnerID, sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active[runnerID] != sessionID {
		return false
	}
	delete(r.active, runnerID)
	return true
}
