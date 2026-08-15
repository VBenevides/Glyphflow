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
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Permissions   []string `json:"permissions"`
	System        bool     `json:"system"`
	AssignedUsers int      `json:"assignedUsers"`
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

func (s *RoleAdminService) Create(name string, permissions []string) error {
	if name == "" {
		return errors.New("role name is required")
	}
	name = platform.NormalizeIdentityKey(name)
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	return s.repository.Create(context.Background(), "role-"+id, name, "", uniqueStrings(permissions))
}
func (s *RoleAdminService) role(identifier string) (store.RoleRecord, bool, error) {
	role, ok, err := s.repository.FindByID(context.Background(), identifier)
	if err == nil && !ok {
		role, ok, err = s.repository.FindByName(context.Background(), identifier)
	}
	return role, ok, err
}
func (s *RoleAdminService) ReplacePermissions(identifier string, permissions []string) error {
	role, ok, err := s.role(identifier)
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
func (s *RoleAdminService) Rename(identifier, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	role, ok, err := s.role(identifier)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("role not found")
	}
	return s.repository.Rename(context.Background(), role.ID, platform.NormalizeIdentityKey(name))
}
func (s *RoleAdminService) Delete(identifier string) error {
	role, ok, err := s.role(identifier)
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
		out = append(out, RoleDefinition{ID: role.ID, Name: role.Name, Permissions: append([]string{}, role.Permissions...), System: role.System, AssignedUsers: role.AssignedUsers})
	}
	return out
}
func (s *RoleAdminService) Assign(user, role string) error {
	definition, ok, err := s.role(role)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("role not found")
	}
	return s.repository.Assign(context.Background(), user, definition.ID, "manual", "admin-api")
}
func (s *RoleAdminService) Unassign(user, role string) error {
	definition, ok, err := s.role(role)
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
			Name        string `json:"name"`
			Permissions []string
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid role request", err)
			return
		}
		if err := s.Roles.Create(in.Name, in.Permissions); err != nil {
			writeError(w, http.StatusBadRequest, "role creation failed", err)
			return
		}
		role, _, _ := s.Roles.role(in.Name)
		writeJSON(w, 201, map[string]string{"id": role.ID, "name": role.Name})
	})))
	mux.Handle("/api/v1/admin/roles/", s.require("roles.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roleID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/roles/")
		if roleID == "" {
			writeJSON(w, 400, map[string]string{"error": "role_id is required"})
			return
		}
		switch r.Method {
		case http.MethodPut:
			var in struct {
				Name        string `json:"name"`
				Permissions []string
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "invalid role request", err)
				return
			}
			if err := s.Roles.Rename(roleID, in.Name); err != nil {
				writeError(w, http.StatusBadRequest, "role rename failed", err)
				return
			}
			if err := s.Roles.ReplacePermissions(roleID, in.Permissions); err != nil {
				writeError(w, http.StatusBadRequest, "role permission update failed", err)
				return
			}
			writeJSON(w, 200, map[string]string{"id": roleID})
		case http.MethodDelete:
			if err := s.Roles.Delete(roleID); err != nil {
				writeError(w, http.StatusBadRequest, "role deletion failed", err)
				return
			}
			writeJSON(w, 204, nil)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})))
}
