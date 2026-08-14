package platform

import (
	"errors"
	"sync"
)

type RunnerCandidate struct {
	ID, SessionID             string
	Enabled, Draining, Online bool
	Capacity, Active          int
	Capabilities              map[string]string
}

type PlacementRequest struct {
	PinnedRunner string
	Required     map[string]string
}

type Placer struct {
	mu     sync.Mutex
	cursor int
}

func (p *Placer) Select(runners []RunnerCandidate, request PlacementRequest) (RunnerCandidate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if request.PinnedRunner != "" {
		for _, runner := range runners {
			if runner.ID == request.PinnedRunner && eligible(runner, request.Required) {
				return runner, nil
			}
		}
		return RunnerCandidate{}, errors.New("pinned runner is unavailable")
	}
	for i := 0; i < len(runners); i++ {
		idx := (p.cursor + i) % len(runners)
		if eligible(runners[idx], request.Required) {
			p.cursor = (idx + 1) % len(runners)
			return runners[idx], nil
		}
	}
	return RunnerCandidate{}, errors.New("no eligible runner")
}

func eligible(r RunnerCandidate, required map[string]string) bool {
	if !r.Enabled || r.Draining || !r.Online || r.SessionID == "" || (r.Capacity > 0 && r.Active >= r.Capacity) {
		return false
	}
	for key, value := range required {
		if r.Capabilities[key] != value {
			return false
		}
	}
	return true
}
