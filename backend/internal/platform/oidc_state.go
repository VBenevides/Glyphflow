package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type AuthorizationStateStore struct {
	mu     sync.Mutex
	states map[string]authorizationState
}

type authorizationState struct {
	Provider string
	Purpose  string
	Nonce    string
	Expires  time.Time
	Used     bool
}

func NewAuthorizationStateStore() *AuthorizationStateStore {
	return &AuthorizationStateStore{states: make(map[string]authorizationState)}
}

func (s *AuthorizationStateStore) Create(provider, purpose string, now time.Time, lifetime time.Duration) (string, string, error) {
	if provider == "" || purpose == "" || lifetime <= 0 {
		return "", "", errors.New("authorization state inputs are required")
	}
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	state := hex.EncodeToString(value)
	nonce := hex.EncodeToString(value[:16])
	s.mu.Lock()
	s.states[hashString(state)] = authorizationState{Provider: provider, Purpose: purpose, Nonce: hashString(nonce), Expires: now.Add(lifetime)}
	s.mu.Unlock()
	return state, nonce, nil
}

func (s *AuthorizationStateStore) Consume(state, nonce, provider, purpose string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := hashString(state)
	entry, ok := s.states[key]
	if !ok || entry.Used || entry.Provider != provider || entry.Purpose != purpose || !now.Before(entry.Expires) || entry.Nonce != hashString(nonce) {
		return errors.New("authorization state is invalid")
	}
	entry.Used = true
	s.states[key] = entry
	return nil
}

func hashString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
