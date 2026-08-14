package api

import (
	"context"
	"errors"
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

func (s *memoryUserRepository) List(_ context.Context) ([]store.UserRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]store.UserRecord, 0, len(s.users))
	for _, user := range s.users {
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
	user.DisplayName = displayName
	s.users[userID] = user
	return nil
}

func (s *memoryUserRepository) SetEnabled(_ context.Context, userID string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.Enabled = enabled
	s.users[userID] = user
	return nil
}
