package platform

import (
	"errors"
	"strings"
	"sync"
)

type RoleAssignment struct {
	UserID       string
	RoleID       string
	SourceType   string
	SourceKey    string
	ProviderID   string
	ExternalID   string
}

type RoleAssignmentStore struct {
	mu    sync.Mutex
	items map[string]RoleAssignment
}

func NewRoleAssignmentStore() *RoleAssignmentStore {
	return &RoleAssignmentStore{items: make(map[string]RoleAssignment)}
}

func CanonicalSourceKey(sourceType, sourceKey string) (string, error) {
	sourceType, sourceKey = NormalizeIdentityKey(sourceType), NormalizeIdentityKey(sourceKey)
	if sourceType == "" || sourceKey == "" {
		return "", errors.New("assignment source type and key are required")
	}
	return sourceType + ":" + sourceKey, nil
}

func (s *RoleAssignmentStore) Add(a RoleAssignment) error {
	if a.UserID == "" || a.RoleID == "" {
		return errors.New("assignment user and role are required")
	}
	key, err := CanonicalSourceKey(a.SourceType, a.SourceKey)
	if err != nil {
		return err
	}
	a.SourceType, a.SourceKey = strings.SplitN(key, ":", 2)[0], strings.SplitN(key, ":", 2)[1]
	identity := a.UserID + "\x00" + a.RoleID + "\x00" + key
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[identity]; exists {
		return errors.New("duplicate role assignment")
	}
	s.items[identity] = a
	return nil
}

func (s *RoleAssignmentStore) List(userID string) []RoleAssignment {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []RoleAssignment
	for _, a := range s.items {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out
}
