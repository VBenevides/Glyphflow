package platform

import (
	"errors"
	"sync"
)

type User struct {
	ID, Username, Email string
	Enabled             bool
}
type Role struct {
	ID, Key string
	System  bool
}
type Permission struct{ ID, Key string }

type AuthStore struct {
	mu              sync.RWMutex
	users           map[string]User
	passwords       map[string]string
	roles           map[string]Role
	permissions     map[string]Permission
	rolePermissions map[string]map[string]bool
	assignments     map[string]map[string]bool
}

func NewAuthStore() *AuthStore {
	return &AuthStore{users: map[string]User{}, passwords: map[string]string{}, roles: map[string]Role{}, permissions: map[string]Permission{}, rolePermissions: map[string]map[string]bool{}, assignments: map[string]map[string]bool{}}
}

func (s *AuthStore) AddUser(user User) error {
	if user.ID == "" || NormalizeIdentityKey(user.Username) == "" {
		return errors.New("user ID and username are required")
	}
	user.Username = NormalizeIdentityKey(user.Username)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.users {
		if NormalizeIdentityKey(existing.Username) == user.Username {
			return errors.New("username already exists")
		}
	}
	if !user.Enabled {
		user.Enabled = true
	}
	s.users[user.ID] = user
	return nil
}
func (s *AuthStore) SetPassword(userID, encoded string) error {
	if userID == "" || encoded == "" {
		return errors.New("password hash is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return errors.New("user not found")
	}
	s.passwords[userID] = encoded
	return nil
}
func (s *AuthStore) AddRole(role Role, permissions ...string) error {
	if role.ID == "" || role.Key == "" {
		return errors.New("role is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[role.ID]; ok {
		return errors.New("role already exists")
	}
	s.roles[role.ID] = role
	s.rolePermissions[role.ID] = map[string]bool{}
	for _, key := range permissions {
		s.rolePermissions[role.ID][key] = true
	}
	return nil
}
func (s *AuthStore) AddPermission(permission Permission) error {
	if permission.ID == "" || permission.Key == "" {
		return errors.New("permission is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permissions[permission.Key] = permission
	return nil
}
func (s *AuthStore) AssignRole(userID, roleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return errors.New("user not found")
	}
	if _, ok := s.roles[roleID]; !ok {
		return errors.New("role not found")
	}
	if s.assignments[userID] == nil {
		s.assignments[userID] = map[string]bool{}
	}
	s.assignments[userID][roleID] = true
	return nil
}
func (s *AuthStore) EffectivePermissions(userID string) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]bool{}
	for role := range s.assignments[userID] {
		for key := range s.rolePermissions[role] {
			out[key] = true
		}
	}
	return out
}
