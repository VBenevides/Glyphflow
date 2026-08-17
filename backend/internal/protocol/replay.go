package protocol

import (
	"errors"
	"sync"
)

type ReplayGuard struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewReplayGuard() *ReplayGuard {
	return &ReplayGuard{seen: make(map[string]struct{})}
}

func (g *ReplayGuard) Accept(messageID string) error {
	if messageID == "" {
		return errors.New("message ID is empty")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.seen[messageID]; exists {
		return errors.New("message replay detected")
	}
	g.seen[messageID] = struct{}{}
	return nil
}
