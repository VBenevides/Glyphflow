package platform

import "errors"

type RunnerSessionRegistry struct {
	active map[string]string
}

func NewRunnerSessionRegistry() *RunnerSessionRegistry {
	return &RunnerSessionRegistry{active: make(map[string]string)}
}

func (r *RunnerSessionRegistry) Connect(runnerID, sessionID string) (string, error) {
	if runnerID == "" || sessionID == "" {
		return "", errors.New("runner and session IDs are required")
	}
	previous := r.active[runnerID]
	r.active[runnerID] = sessionID
	return previous, nil
}

func (r *RunnerSessionRegistry) IsActive(runnerID, sessionID string) bool {
	return r.active[runnerID] == sessionID
}
