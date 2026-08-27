package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type memoryUserRepository struct {
	mu        sync.RWMutex
	users     map[string]store.UserRecord
	byEmail   map[string]string
	passwords map[string]string
}

func newMemoryUserRepository() *memoryUserRepository {
	return &memoryUserRepository{users: map[string]store.UserRecord{}, byEmail: map[string]string{}, passwords: map[string]string{}}
}

func (s *memoryUserRepository) Create(_ context.Context, user store.UserRecord, passwordHash string) error {
	user.DisplayName = store.NormalizeDisplayName(user.Email, user.DisplayName)
	if user.Status == "" {
		if user.Enabled {
			user.Status = store.StatusActive
		} else {
			user.Status = store.StatusDisabled
		}
	}
	if !store.ValidUserStatus(user.Status) {
		return fmt.Errorf("invalid user status %q", user.Status)
	}
	user.Enabled = user.Status == store.StatusActive
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; ok || s.byEmail[user.Email] != "" {
		return errors.New("user already exists")
	}
	s.users[user.ID], s.byEmail[user.Email], s.passwords[user.ID] = user, user.ID, passwordHash
	return nil
}

func (s *memoryUserRepository) FindByID(_ context.Context, id string) (store.UserRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	return user, ok, nil
}

func (s *memoryUserRepository) FindByEmail(_ context.Context, email string) (store.UserRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id := s.byEmail[email]
	user, ok := s.users[id]
	return user, ok, nil
}

func (s *memoryUserRepository) List(_ context.Context, status string) ([]store.UserRecord, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !store.ValidUserStatus(status) {
		return nil, fmt.Errorf("invalid user status %q", status)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]store.UserRecord, 0, len(s.users))
	for _, user := range s.users {
		if status != "" && user.Status != status {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

func (s *memoryUserRepository) PasswordHash(_ context.Context, userID string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hash, ok := s.passwords[userID]
	return hash, ok && hash != "", nil
}

func (s *memoryUserRepository) SetPasswordHash(_ context.Context, userID, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return errors.New("user not found")
	}
	s.passwords[userID] = hash
	return nil
}

func (s *memoryUserRepository) UpdateDisplayName(_ context.Context, userID, displayName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.DisplayName = store.NormalizeDisplayName(user.Email, displayName)
	s.users[userID] = user
	return nil
}

func (s *memoryUserRepository) SetEnabled(_ context.Context, userID string, enabled bool) error {
	status := store.StatusDisabled
	if enabled {
		status = store.StatusActive
	}
	return s.SetStatus(context.Background(), userID, status)
}

func (s *memoryUserRepository) SetStatus(_ context.Context, userID, status string) error {
	if !store.ValidUserStatus(status) {
		return fmt.Errorf("invalid user status %q", status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.Status = status
	user.Enabled = status == store.StatusActive
	s.users[userID] = user
	return nil
}
