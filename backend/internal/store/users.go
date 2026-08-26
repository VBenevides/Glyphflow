package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRecord struct {
	ID, Username, Email, DisplayName string
	Status                           string
	Enabled                          bool
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

type UserStore struct{ pool *pgxpool.Pool }

func NewUserRepository(pool *pgxpool.Pool) *UserStore { return &UserStore{pool: pool} }

func (s *UserStore) Create(ctx context.Context, user UserRecord, passwordHash string) error {
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
	var rows pgx.Rows
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
