package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type RoleDefinition struct {
	Key         string   `json:"key"`
	Permissions []string `json:"permissions"`
	System      bool     `json:"system"`
}
type RoleAdminService struct {
	mu          sync.Mutex
	roles       map[string]RoleDefinition
	assignments map[string]map[string]bool
}

func NewRoleAdminService() *RoleAdminService {
	return &RoleAdminService{roles: map[string]RoleDefinition{}, assignments: map[string]map[string]bool{}}
}
func (s *RoleAdminService) Create(key string, permissions []string) error {
	if key == "" {
		return errors.New("role key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key = platform.NormalizeIdentityKey(key)
	if _, ok := s.roles[key]; ok {
		return errors.New("role exists")
	}
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	s.roles[key] = RoleDefinition{Key: key, Permissions: uniqueStrings(permissions)}
	return nil
}
func (s *RoleAdminService) ReplacePermissions(key string, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	role, ok := s.roles[key]
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	role.Permissions = uniqueStrings(permissions)
	s.roles[key] = role
	return nil
}
func (s *RoleAdminService) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	role, ok := s.roles[key]
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	delete(s.roles, key)
	for user, roles := range s.assignments {
		delete(roles, key)
		if len(roles) == 0 {
			delete(s.assignments, user)
		}
	}
	return nil
}
func (s *RoleAdminService) Seed(key string, permissions []string) error {
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[key]; ok {
		return nil
	}
	s.roles[key] = RoleDefinition{Key: key, Permissions: uniqueStrings(permissions), System: true}
	return nil
}
func (s *RoleAdminService) List() []RoleDefinition {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RoleDefinition, 0, len(s.roles))
	for _, role := range s.roles {
		role.Permissions = append([]string(nil), role.Permissions...)
		out = append(out, role)
	}
	return out
}
func (s *RoleAdminService) Assign(user, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[role]; !ok {
		return errors.New("role not found")
	}
	if s.assignments[user] == nil {
		s.assignments[user] = map[string]bool{}
	}
	s.assignments[user][role] = true
	return nil
}
func (s *RoleAdminService) Unassign(user, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assignments[user] == nil || !s.assignments[user][role] {
		return errors.New("assignment not found")
	}
	delete(s.assignments[user], role)
	return nil
}
func (s *RoleAdminService) Effective(user string) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]bool{}
	for role := range s.assignments[user] {
		for _, permission := range s.roles[role].Permissions {
			out[permission] = true
		}
	}
	return out
}

func validatePermissions(permissions []string) error {
	allowed := map[string]bool{}
	for _, permission := range platform.PermissionCatalog {
		allowed[permission] = true
	}
	for _, permission := range permissions {
		if !allowed[permission] {
			return errors.New("unknown permission: " + permission)
		}
	}
	return nil
}
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
func (s Server) roleRoutes(mux routeRegistrar) {
	if s.Roles == nil {
		return
	}
	mux.Handle("/api/v1/admin/roles", s.require("roles.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, 200, s.Roles.List())
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var in struct {
			Key         string
			Permissions []string
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || s.Roles.Create(in.Key, in.Permissions) != nil {
			writeJSON(w, 400, map[string]string{"error": "role creation failed"})
			return
		}
		writeJSON(w, 201, map[string]string{"key": in.Key})
	})))
	mux.Handle("/api/v1/admin/roles/", s.require("roles.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/roles/")
		if key == "" {
			writeJSON(w, 400, map[string]string{"error": "role key is required"})
			return
		}
		switch r.Method {
		case http.MethodPut:
			var in struct{ Permissions []string }
			if json.NewDecoder(r.Body).Decode(&in) != nil || s.Roles.ReplacePermissions(key, in.Permissions) != nil {
				writeJSON(w, 400, map[string]string{"error": "role update failed"})
				return
			}
			writeJSON(w, 200, map[string]string{"key": key})
		case http.MethodDelete:
			if err := s.Roles.Delete(key); err != nil {
				writeJSON(w, 400, map[string]string{"error": "role deletion failed"})
				return
			}
			writeJSON(w, 204, nil)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})))
}
