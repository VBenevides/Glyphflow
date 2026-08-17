package store

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleRecord struct {
	ID, Name, Description string
	System                bool
	Permissions           []string
	AssignedUsers         int
}

type RoleAssignmentRecord struct {
	UserID, RoleID, SourceType, SourceKey string
}

type RoleRepository interface {
	Ensure(context.Context, string, string, string, bool, []string) error
	Create(context.Context, string, string, string, []string) error
	List(context.Context) ([]RoleRecord, error)
	FindByID(context.Context, string) (RoleRecord, bool, error)
	FindByName(context.Context, string) (RoleRecord, bool, error)
	Rename(context.Context, string, string) error
	ReplacePermissions(context.Context, string, []string) error
	Delete(context.Context, string) error
	Assign(context.Context, string, string, string, string) error
	ReplaceSourceAssignments(context.Context, string, string, []string) error
	ReplaceSSOAssignments(context.Context, string, string, []RoleAssignmentRecord) error
	Unassign(context.Context, string, string) error
	UnassignSource(context.Context, string, string, string) error
	UserRoles(context.Context, string) ([]RoleRecord, []RoleAssignmentRecord, error)
	EffectivePermissions(context.Context, string) ([]string, error)
}

type RoleStore struct{ pool *pgxpool.Pool }

func NewRoleRepository(pool *pgxpool.Pool) *RoleStore { return &RoleStore{pool: pool} }

func SSOAssignmentKey(providerID, groupName string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(providerID)) + "." + base64.RawURLEncoding.EncodeToString([]byte(groupName))
}

func ssoAssignmentPrefix(providerID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(providerID)) + "."
}

func (s *RoleStore) Ensure(ctx context.Context, id, name, description string, system bool, permissions []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO roles (id, name, description, is_system) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, id, name, description, system); err != nil {
		return err
	}
	if err := replaceRolePermissions(ctx, tx, id, permissions); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *RoleStore) Create(ctx context.Context, id, name, description string, permissions []string) error {
	return s.Ensure(ctx, id, name, description, false, permissions)
}

func replaceRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, permissions []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	for _, permission := range uniqueNonEmpty(permissions) {
		permissionID := "permission-" + strings.ReplaceAll(permission, ".", "-")
		if _, err := tx.Exec(ctx, `INSERT INTO permissions (id, name) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`, permissionID, permission); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) SELECT $1, id FROM permissions WHERE name = $2 ON CONFLICT DO NOTHING`, roleID, permission); err != nil {
			return err
		}
	}
	return nil
}

func (s *RoleStore) List(ctx context.Context) ([]RoleRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id, r.name, r.description, r.is_system, COUNT(DISTINCT a.user_id) FROM roles r LEFT JOIN role_assignments a ON a.role_id = r.id GROUP BY r.id, r.name, r.description, r.is_system ORDER BY lower(r.name), r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := []RoleRecord{}
	for rows.Next() {
		var role RoleRecord
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.System, &role.AssignedUsers); err != nil {
			return nil, err
		}
		role.Permissions, err = s.permissions(ctx, role.ID)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *RoleStore) FindByID(ctx context.Context, id string) (RoleRecord, bool, error) {
	return s.find(ctx, `WHERE id = $1`, id)
}

func (s *RoleStore) FindByName(ctx context.Context, name string) (RoleRecord, bool, error) {
	return s.find(ctx, `WHERE lower(name) = lower($1)`, name)
}

func (s *RoleStore) Rename(ctx context.Context, roleID, name string) error {
	var system bool
	if err := s.pool.QueryRow(ctx, `SELECT is_system FROM roles WHERE id = $1`, roleID).Scan(&system); errors.Is(err, pgx.ErrNoRows) {
		return errors.New("role not found")
	} else if err != nil {
		return err
	} else if system {
		return errors.New("system role is immutable")
	}
	_, err := s.pool.Exec(ctx, `UPDATE roles SET name = $2 WHERE id = $1`, roleID, name)
	return err
}

func (s *RoleStore) find(ctx context.Context, clause, value string) (RoleRecord, bool, error) {
	var role RoleRecord
	err := s.pool.QueryRow(ctx, `SELECT id, name, description, is_system FROM roles `+clause, value).Scan(&role.ID, &role.Name, &role.Description, &role.System)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleRecord{}, false, nil
	}
	if err != nil {
		return RoleRecord{}, false, err
	}
	role.Permissions, err = s.permissions(ctx, role.ID)
	return role, true, err
}

func (s *RoleStore) permissions(ctx context.Context, roleID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.name FROM permissions p JOIN role_permissions rp ON rp.permission_id = p.id WHERE rp.role_id = $1 ORDER BY p.name`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permissions := []string{}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (s *RoleStore) ReplacePermissions(ctx context.Context, roleID string, permissions []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var system bool
	if err := tx.QueryRow(ctx, `SELECT is_system FROM roles WHERE id = $1`, roleID).Scan(&system); errors.Is(err, pgx.ErrNoRows) {
		return errors.New("role not found")
	} else if err != nil {
		return err
	} else if system {
		return errors.New("system role is immutable")
	}
	if err := replaceRolePermissions(ctx, tx, roleID, permissions); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *RoleStore) Delete(ctx context.Context, roleID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var system bool
	if err := tx.QueryRow(ctx, `SELECT is_system FROM roles WHERE id = $1`, roleID).Scan(&system); errors.Is(err, pgx.ErrNoRows) {
		return errors.New("role not found")
	} else if err != nil {
		return err
	} else if system {
		return errors.New("system role is immutable")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *RoleStore) Assign(ctx context.Context, userID, roleID, sourceType, sourceKey string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, userID, roleID, sourceType, sourceKey)
	return err
}

func (s *RoleStore) Unassign(ctx context.Context, userID, roleID string) error {
	return s.UnassignSource(ctx, userID, roleID, "manual")
}

func (s *RoleStore) UnassignSource(ctx context.Context, userID, roleID, sourceType string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM role_assignments WHERE user_id = $1 AND role_id = $2 AND source_type = $3`, userID, roleID, sourceType)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("assignment not found")
	}
	return nil
}

func (s *RoleStore) ReplaceSourceAssignments(ctx context.Context, roleID, sourceType string, userIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1)`, roleID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("role not found")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE role_id = $1 AND source_type = $2`, roleID, sourceType); err != nil {
		return err
	}
	for _, userID := range uniqueNonEmpty(userIDs) {
		if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, $3, $1)`, userID, roleID, sourceType); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *RoleStore) ReplaceSSOAssignments(ctx context.Context, userID, providerID string, assignments []RoleAssignmentRecord) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE user_id = $1 AND source_type = 'sso' AND source_key LIKE $2`, userID, ssoAssignmentPrefix(providerID)+"%"); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, assignment := range assignments {
		key := assignment.RoleID + "\x00" + assignment.SourceKey
		if assignment.RoleID == "" || assignment.SourceKey == "" || seen[key] {
			continue
		}
		seen[key] = true
		if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, 'sso', $3) ON CONFLICT DO NOTHING`, userID, assignment.RoleID, SSOAssignmentKey(providerID, assignment.SourceKey)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *RoleStore) UserRoles(ctx context.Context, userID string) ([]RoleRecord, []RoleAssignmentRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id, r.name, r.description, r.is_system, a.source_type, a.source_key FROM role_assignments a JOIN roles r ON r.id = a.role_id WHERE a.user_id = $1 ORDER BY lower(r.name), r.id, a.source_type, a.source_key`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	roles := []RoleRecord{}
	assignments := []RoleAssignmentRecord{}
	seen := map[string]bool{}
	for rows.Next() {
		var role RoleRecord
		var assignment RoleAssignmentRecord
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.System, &assignment.SourceType, &assignment.SourceKey); err != nil {
			return nil, nil, err
		}
		assignment.UserID, assignment.RoleID = userID, role.ID
		assignments = append(assignments, assignment)
		if !seen[role.ID] {
			role.Permissions, err = s.permissions(ctx, role.ID)
			if err != nil {
				return nil, nil, err
			}
			roles, seen[role.ID] = append(roles, role), true
		}
	}
	return roles, assignments, rows.Err()
}

func (s *RoleStore) EffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT p.name FROM role_assignments a JOIN role_permissions rp ON rp.role_id = a.role_id JOIN permissions p ON p.id = rp.permission_id WHERE a.user_id = $1 ORDER BY p.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permissions := []string{}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
