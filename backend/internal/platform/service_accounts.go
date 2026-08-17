package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

type ServiceAccountStore struct {
	mu     sync.Mutex
	hashes map[string]string
}

func NewServiceAccountStore() *ServiceAccountStore {
	return &ServiceAccountStore{hashes: map[string]string{}}
}
func (s *ServiceAccountStore) Issue(id string) (string, error) {
	if id == "" {
		return "", errors.New("service account ID is required")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	s.hashes[id] = hex.EncodeToString(digest[:])
	s.mu.Unlock()
	return token, nil
}
func (s *ServiceAccountStore) Verify(id, token string) bool {
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hashes[id] == hex.EncodeToString(digest[:])
}
