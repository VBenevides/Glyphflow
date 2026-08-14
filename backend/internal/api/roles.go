package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type RoleDefinition struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Permissions []string `json:"permissions"`
	System      bool     `json:"system"`
}
type RoleAdminService struct {
	repository store.RoleRepository
}

func NewRoleAdminService() *RoleAdminService {
	return &RoleAdminService{repository: newMemoryRoleRepository()}
}

func (s *RoleAdminService) SetRepository(repository store.RoleRepository) {
	if repository != nil {
		s.repository = repository
	}
}

func systemRoleID(name string) string {
	switch platform.NormalizeIdentityKey(name) {
	case "admin":
		return "system-admin"
	case "user":
		return "system-user"
	default:
		return "system-" + platform.NormalizeIdentityKey(name)
	}
}

func (s *RoleAdminService) Create(key string, permissions []string) error {
	if key == "" {
		return errors.New("role key is required")
	}
	key = platform.NormalizeIdentityKey(key)
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	return s.repository.Create(context.Background(), "role-"+id, key, "", uniqueStrings(permissions))
}
func (s *RoleAdminService) ReplacePermissions(key string, permissions []string) error {
	role, ok, err := s.repository.FindByName(context.Background(), key)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	return s.repository.ReplacePermissions(context.Background(), role.ID, uniqueStrings(permissions))
}
func (s *RoleAdminService) Delete(key string) error {
	role, ok, err := s.repository.FindByName(context.Background(), key)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	return s.repository.Delete(context.Background(), role.ID)
}
func (s *RoleAdminService) Seed(key string, permissions []string) error {
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	return s.repository.Ensure(context.Background(), systemRoleID(key), platform.NormalizeIdentityKey(key), "", true, uniqueStrings(permissions))
}
func (s *RoleAdminService) List() []RoleDefinition {
	roles, err := s.repository.List(context.Background())
	if err != nil {
		return []RoleDefinition{}
	}
	out := make([]RoleDefinition, 0, len(roles))
	for _, role := range roles {
		out = append(out, RoleDefinition{ID: role.ID, Key: role.Name, Permissions: append([]string{}, role.Permissions...), System: role.System})
	}
	return out
}
func (s *RoleAdminService) Assign(user, role string) error {
	definition, ok, err := s.repository.FindByName(context.Background(), role)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("role not found")
	}
	return s.repository.Assign(context.Background(), user, definition.ID, "manual", "admin-api")
}
func (s *RoleAdminService) Unassign(user, role string) error {
	definition, ok, err := s.repository.FindByName(context.Background(), role)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("assignment not found")
	}
	return s.repository.Unassign(context.Background(), user, definition.ID)
}
func (s *RoleAdminService) Effective(user string) map[string]bool {
	out := map[string]bool{}
	permissions, err := s.repository.EffectivePermissions(context.Background(), user)
	if err != nil {
		return out
	}
	for _, permission := range permissions {
		out[permission] = true
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
	mux.Handle("/api/v1/admin/roles", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "roles.read|roles.manage"
		}
		return "roles.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
