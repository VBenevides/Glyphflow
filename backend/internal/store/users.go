package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

type UserRecord struct {
	ID, Username, Email, DisplayName string
	Status                           string
	Enabled                          bool
}

func DefaultUserDisplayName(email string) string {
	local := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	parts := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '-' })
	for i, part := range parts {
		word := []rune(strings.ToLower(part))
		if len(word) > 0 {
			word[0] = unicode.ToUpper(word[0])
			parts[i] = string(word)
		}
	}
	if len(parts) == 0 {
		return "User"
	}
	return strings.Join(parts, " ")
}

func NormalizeDisplayName(email, displayName string) string {
	if displayName = strings.TrimSpace(displayName); displayName != "" {
		return displayName
	}
	return DefaultUserDisplayName(email)
}

const (
	StatusActive   = "active"
	StatusPending  = "pending"
	StatusDisabled = "disabled"
)

func ValidUserStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusActive, StatusPending, StatusDisabled:
		return true
	default:
		return false
	}
}

func userStatus(user UserRecord) (string, error) {
	status := strings.ToLower(strings.TrimSpace(user.Status))
	if status == "" {
		if user.Enabled {
			return StatusActive, nil
		}
		return StatusDisabled, nil
	}
	if !ValidUserStatus(status) {
		return "", fmt.Errorf("invalid user status %q", user.Status)
	}
	return status, nil
}

type UserRepository interface {
	Create(context.Context, UserRecord, string) error
	FindByID(context.Context, string) (UserRecord, bool, error)
	FindByEmail(context.Context, string) (UserRecord, bool, error)
	List(context.Context, string) ([]UserRecord, error)
	PasswordHash(context.Context, string) (string, bool, error)
	SetPasswordHash(context.Context, string, string) error
	UpdateDisplayName(context.Context, string, string) error
	SetEnabled(context.Context, string, bool) error
	SetStatus(context.Context, string, string) error
}

type UserStore struct{ pool database }

func NewUserRepository(pool any) *UserStore {
	db, _ := databaseFrom(pool)
	return &UserStore{pool: db}
}

func (s *UserStore) Create(ctx context.Context, user UserRecord, passwordHash string) error {
	user.DisplayName = NormalizeDisplayName(user.Email, user.DisplayName)
	status, err := userStatus(user)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, username, email, display_name, status) VALUES ($1, $2, $3, $4, $5)`, user.ID, user.Username, user.Email, user.DisplayName, status); err != nil {
		return err
	}
	if passwordHash != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO user_passwords (user_id, password_hash) VALUES ($1, $2)`, user.ID, passwordHash); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *UserStore) ProvisionLocal(ctx context.Context, user UserRecord, passwordHash, defaultRoleID, adminRoleID string) error {
	user.DisplayName = NormalizeDisplayName(user.Email, user.DisplayName)
	status, err := userStatus(user)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, username, email, display_name, status) VALUES ($1, $2, $3, $4, $5)`, user.ID, user.Username, user.Email, user.DisplayName, status); err != nil {
		return err
	}
	if passwordHash != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO user_passwords (user_id, password_hash) VALUES ($1, $2)`, user.ID, passwordHash); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, 'default', $2)`, user.ID, defaultRoleID); err != nil {
		return err
	}
	if adminRoleID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, 'system-admin', $1)`, user.ID, adminRoleID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *UserStore) FindByID(ctx context.Context, id string) (UserRecord, bool, error) {
	return s.find(ctx, `WHERE id = $1`, id)
}

func (s *UserStore) FindByEmail(ctx context.Context, email string) (UserRecord, bool, error) {
	return s.find(ctx, `WHERE lower(email) = lower($1)`, email)
}

func (s *UserStore) find(ctx context.Context, clause, value string) (UserRecord, bool, error) {
	var user UserRecord
	var status string
	err := s.pool.QueryRow(ctx, `SELECT id, username, email, display_name, status FROM users `+clause, value).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	user.Status = status
	user.Enabled = status == StatusActive
	return user, true, nil
}

func (s *UserStore) List(ctx context.Context, status string) ([]UserRecord, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !ValidUserStatus(status) {
		return nil, fmt.Errorf("invalid user status %q", status)
	}
	query := `SELECT id, username, email, display_name, status FROM users`
	if status != "" {
		query += ` WHERE status = $1`
	}
	query += ` ORDER BY lower(username), id`
	var rows databaseRows
	var err error
	if status == "" {
		rows, err = s.pool.Query(ctx, query)
	} else {
		rows, err = s.pool.Query(ctx, query, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []UserRecord{}
	for rows.Next() {
		var user UserRecord
		var status string
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &status); err != nil {
			return nil, err
		}
		user.Status = status
		user.Enabled = status == StatusActive
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *UserStore) ListPage(ctx context.Context, status, email string, roles []string, limit, offset int) ([]UserRecord, int, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !ValidUserStatus(status) {
		return nil, 0, fmt.Errorf("invalid user status %q", status)
	}
	if limit < 1 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	conditions := []string{"1 = 1"}
	args := []any{}
	if status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("u.status = $%d", len(args)))
	}
	if email = strings.TrimSpace(email); email != "" {
		args = append(args, email)
		conditions = append(conditions, fmt.Sprintf("lower(u.email) LIKE '%%' || lower($%d) || '%%'", len(args)))
	}
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		args = append(args, role)
		conditions = append(conditions, fmt.Sprintf(`EXISTS (SELECT 1 FROM role_assignments a JOIN roles r ON r.id = a.role_id WHERE a.user_id = u.id AND lower(r.name) = $%d)`, len(args)))
	}
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, limit, offset)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT u.id, u.username, u.email, u.display_name, u.status, count(*) OVER()
		FROM users u
		WHERE %s
		ORDER BY lower(u.username), u.id
		LIMIT $%d OFFSET $%d`, strings.Join(conditions, " AND "), limitArg, offsetArg), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := []UserRecord{}
	total := 0
	for rows.Next() {
		var user UserRecord
		var userStatus string
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &userStatus, &total); err != nil {
			return nil, 0, err
		}
		user.Status = userStatus
		user.Enabled = userStatus == StatusActive
		users = append(users, user)
	}
	return users, total, rows.Err()
}

func (s *UserStore) PasswordHash(ctx context.Context, userID string) (string, bool, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM user_passwords WHERE user_id = $1`, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

func (s *UserStore) SetPasswordHash(ctx context.Context, userID, hash string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_passwords (user_id, password_hash) VALUES ($1, $2) ON CONFLICT (user_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, password_changed_at = now()`, userID, hash)
	return err
}

func (s *UserStore) UpdateDisplayName(ctx context.Context, userID, displayName string) error {
	if strings.TrimSpace(displayName) == "" {
		var email string
		if err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
			return err
		}
		displayName = DefaultUserDisplayName(email)
	}
	displayName = strings.TrimSpace(displayName)
	_, err := s.pool.Exec(ctx, `UPDATE users SET display_name = $2, updated_at = now() WHERE id = $1`, userID, displayName)
	return err
}

func (s *UserStore) SetEnabled(ctx context.Context, userID string, enabled bool) error {
	return s.SetStatus(ctx, userID, enabledStatus(enabled))
}

func (s *UserStore) SetStatus(ctx context.Context, userID, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if !ValidUserStatus(status) {
		return fmt.Errorf("invalid user status %q", status)
	}
	_, err := s.pool.Exec(ctx, `UPDATE users SET status = $2, updated_at = now() WHERE id = $1`, userID, status)
	return err
}

func enabledStatus(enabled bool) string {
	if enabled {
		return "active"
	}
	return "disabled"
}
