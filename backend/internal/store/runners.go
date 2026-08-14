package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunnerRecord struct {
	ID, Name, Pool, DesiredState, ObservedState, Platform, Architecture string
	Capacity, ActiveCount                                               int
	HeartbeatAt                                                         *time.Time
}

type RunnerEnrollmentRecord struct {
	ID, RunnerID, TokenHash, Requester, Target string
	ExpiresAt                                  time.Time
	Artifact                                   map[string]any
}

type RunnerRepository interface {
	EnsurePool(context.Context, string, string) error
	List(context.Context) ([]RunnerRecord, error)
	Find(context.Context, string) (RunnerRecord, bool, error)
	SetDesiredState(context.Context, string, string) (RunnerRecord, bool, error)
	CreateEnrollment(context.Context, RunnerRecord, RunnerEnrollmentRecord) error
	ConsumeEnrollment(context.Context, string, time.Time) (RunnerRecord, error)
	CreateSession(context.Context, string, string) error
	Heartbeat(context.Context, string, time.Time) error
}

type RunnerStore struct{ pool *pgxpool.Pool }

func NewRunnerRepository(pool *pgxpool.Pool) *RunnerStore { return &RunnerStore{pool: pool} }

func (s *RunnerStore) EnsurePool(ctx context.Context, id, name string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, id, name)
	return err
}

const runnerQuery = `SELECT r.id, r.name, p.name, r.desired_state, r.observed_state, r.capacity, r.active_count, r.last_seen_at, COALESCE(r.capabilities->>'platform', ''), COALESCE(r.capabilities->>'architecture', '') FROM runners r JOIN runner_pools p ON p.id = r.pool_id`

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
	if err := row.Scan(&item.ID, &item.Name, &item.Pool, &item.DesiredState, &item.ObservedState, &item.Capacity, &item.ActiveCount, &item.HeartbeatAt, &item.Platform, &item.Architecture); err != nil {
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
	if _, err := tx.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, observed_state, capacity, capabilities) VALUES ($1, (SELECT id FROM runner_pools WHERE name = $2), $3, 'ENABLED', 'PENDING', $4, $5::jsonb) ON CONFLICT (id) DO NOTHING`, runner.ID, runner.Pool, runner.Name, maxRunnerCapacity(runner.Capacity), artifact); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runner_enrollments (id, runner_id, token_hash, expires_at, requester, target, artifact) VALUES ($1, $2, decode($3, 'hex'), $4, $5, $6, $7::jsonb)`, enrollment.ID, runner.ID, enrollment.TokenHash, enrollment.ExpiresAt, enrollment.Requester, enrollment.Target, artifact); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func maxRunnerCapacity(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func (s *RunnerStore) ConsumeEnrollment(ctx context.Context, tokenHash string, now time.Time) (RunnerRecord, error) {
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
	if _, err := tx.Exec(ctx, `UPDATE runners SET observed_state = 'ONLINE', last_seen_at = $2, updated_at = now() WHERE id = $1`, runnerID, now); err != nil {
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
	_, err := s.pool.Exec(ctx, `UPDATE runners SET observed_state = 'ONLINE', last_seen_at = $2, updated_at = now() WHERE id = $1`, runnerID, now)
	return err
}
