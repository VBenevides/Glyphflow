package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSessionInvalid = errors.New("session is invalid")
	ErrSessionReplay  = errors.New("refresh token replay detected")
)

type SessionRecord struct {
	ID, UserID, RefreshTokenHash, SessionFamilyID string
	AccessExpiresAt, RefreshExpiresAt, LastSeenAt time.Time
	UserAgent, IPAddress                          string
	RevokedAt                                     *time.Time
}

type AdminSessionRecord struct {
	ID, UserID, UserEmail string
	ExpiresAt, LastSeenAt time.Time
	UserAgent, IPAddress  string
}

type SessionRepository interface {
	Create(context.Context, SessionRecord) error
	Get(context.Context, string) (SessionRecord, bool, error)
	Rotate(context.Context, string, string, SessionRecord) error
	Active(context.Context, string, string) (bool, error)
	Revoke(context.Context, string) error
	RevokeFamily(context.Context, string) error
	RevokeUser(context.Context, string) error
	List(context.Context, string) ([]SessionRecord, error)
	DeleteOlderThan(context.Context, time.Time) error
}

type SessionStore struct{ pool database }

func NewSessionRepository(pool any) *SessionStore {
	db, _ := databaseFrom(pool)
	return &SessionStore{pool: db}
}

func (s *SessionStore) Create(ctx context.Context, session SessionRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO auth_sessions (id, user_id, refresh_token_hash, access_expires_at, refresh_expires_at, session_family_id, user_agent, ip_address, last_seen_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, session.ID, session.UserID, session.RefreshTokenHash, session.AccessExpiresAt, session.RefreshExpiresAt, session.SessionFamilyID, session.UserAgent, session.IPAddress, session.LastSeenAt)
	return err
}

func (s *SessionStore) Get(ctx context.Context, id string) (SessionRecord, bool, error) {
	return s.get(ctx, `WHERE id = $1`, id)
}

func (s *SessionStore) get(ctx context.Context, clause string, value string) (SessionRecord, bool, error) {
	var session SessionRecord
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, refresh_token_hash, access_expires_at, refresh_expires_at, session_family_id, user_agent, ip_address, last_seen_at, revoked_at FROM auth_sessions `+clause, value).Scan(&session.ID, &session.UserID, &session.RefreshTokenHash, &session.AccessExpiresAt, &session.RefreshExpiresAt, &session.SessionFamilyID, &session.UserAgent, &session.IPAddress, &session.LastSeenAt, &session.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRecord{}, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, err
	}
	return session, true, nil
}

func (s *SessionStore) Rotate(ctx context.Context, id, refreshTokenHash string, replacement SessionRecord) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current SessionRecord
	err = tx.QueryRow(ctx, `SELECT id, user_id, refresh_token_hash, access_expires_at, refresh_expires_at, session_family_id, user_agent, ip_address, last_seen_at, revoked_at FROM auth_sessions WHERE id = $1 FOR UPDATE`, id).Scan(&current.ID, &current.UserID, &current.RefreshTokenHash, &current.AccessExpiresAt, &current.RefreshExpiresAt, &current.SessionFamilyID, &current.UserAgent, &current.IPAddress, &current.LastSeenAt, &current.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionInvalid
	}
	if err != nil {
		return err
	}
	if current.RevokedAt != nil || !time.Now().Before(current.RefreshExpiresAt) {
		_, _ = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now()) WHERE session_family_id = $1`, current.SessionFamilyID)
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return ErrSessionReplay
	}
	if current.RefreshTokenHash != refreshTokenHash {
		_, _ = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now()) WHERE session_family_id = $1`, current.SessionFamilyID)
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return ErrSessionReplay
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at = now(), last_seen_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO auth_sessions (id, user_id, refresh_token_hash, access_expires_at, refresh_expires_at, session_family_id, user_agent, ip_address, last_seen_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, replacement.ID, current.UserID, replacement.RefreshTokenHash, replacement.AccessExpiresAt, replacement.RefreshExpiresAt, current.SessionFamilyID, current.UserAgent, current.IPAddress, replacement.LastSeenAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *SessionStore) Active(ctx context.Context, id, userID string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM auth_sessions WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND access_expires_at > now())`, id, userID).Scan(&active)
	return active, err
}

func (s *SessionStore) Revoke(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now()), last_seen_at = now() WHERE id = $1`, id)
	return err
}

func (s *SessionStore) RevokeFamily(ctx context.Context, familyID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now()), last_seen_at = now() WHERE session_family_id = $1`, familyID)
	return err
}

func (s *SessionStore) RevokeUser(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now()), last_seen_at = now() WHERE user_id = $1`, userID)
	return err
}

func (s *SessionStore) List(ctx context.Context, userID string) ([]SessionRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, refresh_token_hash, access_expires_at, refresh_expires_at, session_family_id, user_agent, ip_address, last_seen_at, revoked_at FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SessionRecord{}
	for rows.Next() {
		var session SessionRecord
		if err := rows.Scan(&session.ID, &session.UserID, &session.RefreshTokenHash, &session.AccessExpiresAt, &session.RefreshExpiresAt, &session.SessionFamilyID, &session.UserAgent, &session.IPAddress, &session.LastSeenAt, &session.RevokedAt); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, rows.Err()
}

func (s *SessionStore) ListAdminPage(ctx context.Context, email string, limit, offset int) ([]AdminSessionRecord, int, error) {
	if limit < 1 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.user_id, u.email, a.access_expires_at, a.last_seen_at, a.user_agent, a.ip_address, count(*) OVER()
		FROM auth_sessions a
		JOIN users u ON u.id = a.user_id
		WHERE a.revoked_at IS NULL AND ($1 = '' OR lower(u.email) LIKE '%' || lower($1) || '%')
		ORDER BY u.email, a.id
		LIMIT $2 OFFSET $3`, strings.TrimSpace(email), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	sessions := []AdminSessionRecord{}
	total := 0
	for rows.Next() {
		var session AdminSessionRecord
		if err := rows.Scan(&session.ID, &session.UserID, &session.UserEmail, &session.ExpiresAt, &session.LastSeenAt, &session.UserAgent, &session.IPAddress, &total); err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, session)
	}
	return sessions, total, rows.Err()
}

func (s *SessionStore) DeleteOlderThan(ctx context.Context, cutoff time.Time) error {
	if cutoff.IsZero() {
		return errors.New("session cleanup cutoff is required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE created_at < $1`, cutoff)
	return err
}
