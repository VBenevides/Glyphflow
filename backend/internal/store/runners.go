package store

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunnerRecord struct {
	ID, Name, PoolID, Pool, DesiredState, ObservedState, Platform, Architecture string
	Capacity, ActiveCount                                                       int
	HeartbeatAt                                                                 *time.Time
}

type RunnerPoolRecord struct {
	ID, Name, Description string
	Enabled               bool
}

type RunnerEnrollmentRecord struct {
	ID, RunnerID, TokenHash, Requester, Target string
	ExpiresAt                                  time.Time
	Artifact                                   map[string]any
}

type RunnerRepository interface {
	EnsurePool(context.Context, string, string) error
	ListPools(context.Context) ([]RunnerPoolRecord, error)
	FindPool(context.Context, string) (RunnerPoolRecord, bool, error)
	CreatePool(context.Context, RunnerPoolRecord) error
	UpdatePool(context.Context, RunnerPoolRecord) (RunnerPoolRecord, bool, error)
	DeletePool(context.Context, string) error
	List(context.Context) ([]RunnerRecord, error)
	Find(context.Context, string) (RunnerRecord, bool, error)
	SetDesiredState(context.Context, string, string) (RunnerRecord, bool, error)
	Delete(context.Context, string) (bool, error)
	CreateEnrollment(context.Context, RunnerRecord, RunnerEnrollmentRecord) error
	ConsumeEnrollment(context.Context, string, time.Time) (RunnerRecord, error)
	ConsumeEnrollmentWithKey(context.Context, string, time.Time, string, []byte) (RunnerRecord, error)
	CreateSession(context.Context, string, string) error
	Heartbeat(context.Context, string, time.Time) error
	HeartbeatWithKey(context.Context, string, string, time.Time, string, []byte) error
	FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error)
	MarkStale(context.Context, time.Time) error
}

type RunnerStore struct{ pool *pgxpool.Pool }

const DefaultRunnerCapacity = 10

func NewRunnerRepository(pool *pgxpool.Pool) *RunnerStore { return &RunnerStore{pool: pool} }

func (s *RunnerStore) EnsurePool(ctx context.Context, id, name string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, id, name)
	return err
}

func (s *RunnerStore) ListPools(ctx context.Context) ([]RunnerPoolRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, description, enabled FROM runner_pools ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RunnerPoolRecord{}
	for rows.Next() {
		var item RunnerPoolRecord
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Enabled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *RunnerStore) FindPool(ctx context.Context, id string) (RunnerPoolRecord, bool, error) {
	var item RunnerPoolRecord
	err := s.pool.QueryRow(ctx, `SELECT id, name, description, enabled FROM runner_pools WHERE id = $1`, id).Scan(&item.ID, &item.Name, &item.Description, &item.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunnerPoolRecord{}, false, nil
	}
	return item, err == nil, err
}

func (s *RunnerStore) CreatePool(ctx context.Context, item RunnerPoolRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO runner_pools (id, name, description, enabled) VALUES ($1, $2, $3, $4)`, item.ID, item.Name, item.Description, item.Enabled)
	return err
}

func (s *RunnerStore) UpdatePool(ctx context.Context, item RunnerPoolRecord) (RunnerPoolRecord, bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE runner_pools SET name = $2, description = $3, enabled = $4, updated_at = now() WHERE id = $1`, item.ID, item.Name, item.Description, item.Enabled)
	if err != nil || result.RowsAffected() == 0 {
		return RunnerPoolRecord{}, result.RowsAffected() > 0, err
	}
	return s.FindPool(ctx, item.ID)
}

func (s *RunnerStore) DeletePool(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("runner pool not found")
	}
	return nil
}

const runnerQuery = `SELECT r.id, r.name, p.id, p.name, r.desired_state, r.observed_state, r.capacity, r.active_count, r.last_seen_at, COALESCE(r.capabilities->>'platform', ''), COALESCE(r.capabilities->>'architecture', '') FROM runners r JOIN runner_pools p ON p.id = r.pool_id`

func (s *RunnerStore) List(ctx context.Context) ([]RunnerRecord, error) {
	rows, err := s.pool.Query(ctx, runnerQuery+` ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RunnerRecord{}
	for rows.Next() {
		item, err := scanRunner(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *RunnerStore) Find(ctx context.Context, id string) (RunnerRecord, bool, error) {
	item, err := scanRunner(s.pool.QueryRow(ctx, runnerQuery+` WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return RunnerRecord{}, false, nil
	}
	return item, err == nil, err
}

func scanRunner(row interface{ Scan(...any) error }) (RunnerRecord, error) {
	var item RunnerRecord
	if err := row.Scan(&item.ID, &item.Name, &item.PoolID, &item.Pool, &item.DesiredState, &item.ObservedState, &item.Capacity, &item.ActiveCount, &item.HeartbeatAt, &item.Platform, &item.Architecture); err != nil {
		return RunnerRecord{}, err
	}
	return item, nil
}

func (s *RunnerStore) SetDesiredState(ctx context.Context, id, state string) (RunnerRecord, bool, error) {
	query := `UPDATE runners SET desired_state = $2, updated_at = now() WHERE id = $1`
	if state == "REVOKED" {
		query = `UPDATE runners SET desired_state = 'DISABLED', observed_state = 'REVOKED', updated_at = now() WHERE id = $1`
	}
	var result pgconn.CommandTag
	var err error
	if state == "REVOKED" {
		result, err = s.pool.Exec(ctx, query, id)
	} else {
		result, err = s.pool.Exec(ctx, query, id, state)
	}
	if err != nil || result.RowsAffected() == 0 {
		return RunnerRecord{}, result.RowsAffected() > 0, err
	}
	item, found, err := s.Find(ctx, id)
	return item, found, err
}

func (s *RunnerStore) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM runners WHERE id = $1`, id)
	return result.RowsAffected() > 0, err
}

func (s *RunnerStore) CreateEnrollment(ctx context.Context, runner RunnerRecord, enrollment RunnerEnrollmentRecord) error {
	artifact, err := json.Marshal(enrollment.Artifact)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	poolID := runner.PoolID
	if poolID == "" {
		poolID = runner.Pool
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, observed_state, capacity, capabilities) VALUES ($1, (SELECT id FROM runner_pools WHERE id = $2 OR name = $2), $3, 'ENABLED', 'PENDING', CASE WHEN $4 > 0 THEN $4 ELSE 10 END, $5::jsonb) ON CONFLICT (id) DO UPDATE SET capacity = CASE WHEN $4 > 0 THEN EXCLUDED.capacity ELSE runners.capacity END`, runner.ID, poolID, runner.Name, runner.Capacity, artifact); err != nil {
		return err
	}
	var lockedRunnerID string
	if err := tx.QueryRow(ctx, `SELECT id FROM runners WHERE id = $1 FOR UPDATE`, runner.ID).Scan(&lockedRunnerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM runner_enrollments WHERE runner_id = $1 AND used_at IS NULL`, runner.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runner_enrollments (id, runner_id, token_hash, expires_at, requester, target, artifact) VALUES ($1, $2, decode($3, 'hex'), $4, $5, $6, $7::jsonb)`, enrollment.ID, runner.ID, enrollment.TokenHash, enrollment.ExpiresAt, enrollment.Requester, enrollment.Target, artifact); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func maxRunnerCapacity(value int) int {
	if value <= 0 {
		return DefaultRunnerCapacity
	}
	return value
}

func (s *RunnerStore) ConsumeEnrollment(ctx context.Context, tokenHash string, now time.Time) (RunnerRecord, error) {
	return s.consumeEnrollment(ctx, tokenHash, now, "", nil)
}

func (s *RunnerStore) ConsumeEnrollmentWithKey(ctx context.Context, tokenHash string, now time.Time, keyID string, publicKey []byte) (RunnerRecord, error) {
	if keyID == "" || len(publicKey) != ed25519.PublicKeySize {
		return RunnerRecord{}, errors.New("runner enrollment key is incomplete")
	}
	return s.consumeEnrollment(ctx, tokenHash, now, keyID, publicKey)
}

func (s *RunnerStore) consumeEnrollment(ctx context.Context, tokenHash string, now time.Time, keyID string, publicKey []byte) (RunnerRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RunnerRecord{}, err
	}
	defer tx.Rollback(ctx)
	var runnerID string
	var expires time.Time
	var usedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT runner_id, expires_at, used_at FROM runner_enrollments WHERE token_hash = decode($1, 'hex') FOR UPDATE`, tokenHash).Scan(&runnerID, &expires, &usedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunnerRecord{}, errors.New("enrollment not found")
	}
	if err != nil {
		return RunnerRecord{}, err
	}
	if usedAt != nil {
		return RunnerRecord{}, errors.New("enrollment already used")
	}
	if !now.Before(expires) {
		return RunnerRecord{}, errors.New("enrollment expired")
	}
	if _, err := tx.Exec(ctx, `UPDATE runner_enrollments SET used_at = now() WHERE token_hash = decode($1, 'hex')`, tokenHash); err != nil {
		return RunnerRecord{}, err
	}
	if keyID != "" {
		var existingRunner string
		var existingKey []byte
		err := tx.QueryRow(ctx, `SELECT runner_id, public_key FROM runner_keys WHERE key_id = $1 FOR UPDATE`, keyID).Scan(&existingRunner, &existingKey)
		if !errors.Is(err, pgx.ErrNoRows) && err != nil {
			return RunnerRecord{}, err
		}
		if err == nil && existingRunner != runnerID {
			return RunnerRecord{}, errors.New("runner enrollment key is already bound")
		}
		if err == nil {
			if _, err := tx.Exec(ctx, `UPDATE runner_keys SET public_key = $2, not_before = now(), not_after = NULL, revoked_at = NULL WHERE key_id = $1`, keyID, publicKey); err != nil {
				return RunnerRecord{}, err
			}
		} else if _, err := tx.Exec(ctx, `INSERT INTO runner_keys (key_id, runner_id, public_key, not_before) VALUES ($1, $2, $3, now())`, keyID, runnerID, publicKey); err != nil {
			return RunnerRecord{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE runners SET observed_state = 'PENDING', last_seen_at = NULL, updated_at = now() WHERE id = $1`, runnerID); err != nil {
		return RunnerRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RunnerRecord{}, err
	}
	item, found, err := s.Find(ctx, runnerID)
	if err != nil || !found {
		return RunnerRecord{}, errors.New("runner not found")
	}
	return item, nil
}

func (s *RunnerStore) CreateSession(ctx context.Context, runnerID, bootID string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id) VALUES ($1, $2, $3)`, runnerID+"/"+bootID, runnerID, bootID)
	return err
}

func (s *RunnerStore) Heartbeat(ctx context.Context, runnerID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE runners SET observed_state = 'ONLINE', last_seen_at = $2, updated_at = now() WHERE id = $1 AND observed_state <> 'REVOKED' AND (last_seen_at IS NULL OR last_seen_at < $2)`, runnerID, now)
	return err
}

func (s *RunnerStore) HeartbeatWithKey(ctx context.Context, runnerID, bootID string, now time.Time, keyID string, publicKey []byte) error {
	if runnerID == "" || bootID == "" || keyID == "" || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("runner heartbeat key is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE runner_sessions SET disconnected_at = $2 WHERE runner_id = $1 AND disconnected_at IS NULL AND boot_id <> $3`, runnerID, now, bootID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id, last_heartbeat_at) VALUES ($1, $2, $3, $4) ON CONFLICT (runner_id, boot_id) DO UPDATE SET last_heartbeat_at = EXCLUDED.last_heartbeat_at, disconnected_at = NULL`, runnerID+"/"+bootID, runnerID, bootID, now); err != nil {
		return err
	}
	var boundRunner string
	var boundKey []byte
	if err := tx.QueryRow(ctx, `SELECT runner_id, public_key FROM runner_keys WHERE key_id = $1 AND revoked_at IS NULL`, keyID).Scan(&boundRunner, &boundKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("runner heartbeat key is not enrolled")
		}
		return err
	}
	if boundRunner != runnerID || !ed25519.PublicKey(boundKey).Equal(ed25519.PublicKey(publicKey)) {
		return errors.New("runner heartbeat key does not match enrollment")
	}
	if _, err := tx.Exec(ctx, `UPDATE runners SET observed_state = 'ONLINE', last_seen_at = $2, updated_at = now() WHERE id = $1 AND observed_state <> 'REVOKED'`, runnerID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *RunnerStore) FindPublicKey(ctx context.Context, runnerID, keyID string) (ed25519.PublicKey, error) {
	var publicKey []byte
	err := s.pool.QueryRow(ctx, `SELECT public_key FROM runner_keys WHERE runner_id = $1 AND key_id = $2 AND revoked_at IS NULL AND not_before <= now() AND (not_after IS NULL OR not_after > now())`, runnerID, keyID).Scan(&publicKey)
	if err != nil {
		return nil, err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("runner public key is invalid")
	}
	return ed25519.PublicKey(publicKey), nil
}

func (s *RunnerStore) MarkStale(ctx context.Context, cutoff time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE runners SET observed_state = 'OFFLINE', updated_at = now() WHERE observed_state = 'ONLINE' AND (last_seen_at IS NULL OR last_seen_at < $1 OR NOT EXISTS (SELECT 1 FROM runner_sessions WHERE runner_sessions.runner_id = runners.id AND runner_sessions.disconnected_at IS NULL))`, cutoff)
	return err
}
