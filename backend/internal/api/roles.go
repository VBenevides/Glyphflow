package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
)

type RoleAdminService struct {
	mu          sync.Mutex
	roles       map[string]map[string]bool
	system      map[string]bool
	assignments map[string]map[string]bool
}

func NewRoleAdminService() *RoleAdminService {
	return &RoleAdminService{roles: map[string]map[string]bool{}, system: map[string]bool{}, assignments: map[string]map[string]bool{}}
}
func (s *RoleAdminService) Create(key string, permissions []string) error {
	if key == "" {
		return errors.New("role key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[key]; ok {
		return errors.New("role exists")
	}
	s.roles[key] = map[string]bool{}
	for _, permission := range permissions {
		s.roles[key][permission] = true
	}
	return nil
}
func (s *RoleAdminService) ReplacePermissions(key string, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.system[key] {
		return errors.New("system role is immutable")
	}
	if _, ok := s.roles[key]; !ok {
		return errors.New("role not found")
	}
	next := map[string]bool{}
	for _, permission := range permissions {
		next[permission] = true
	}
	s.roles[key] = next
	return nil
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
func (s Server) roleRoutes(mux *http.ServeMux) {
	if s.Roles == nil {
		return
	}
	mux.Handle("/api/v1/admin/roles", s.require("roles.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}
