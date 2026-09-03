package store

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type RunnerRecord struct {
	ID, Name, PoolID, Pool, DesiredState, ObservedState, Platform, Architecture string
	NATSEndpoint, ControlPlaneURL                                               string
	Capacity, CurrentCapacity, ActiveCount                                      int
	HeartbeatAt                                                                 *time.Time
	CurrentMetrics                                                              *RunnerMetricsRecord
	IsArchived, IsDeleted                                                       bool
}

type RunnerPoolRecord struct {
	ID, Name, Description string
	Enabled               bool
	IsDeleted             bool
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
	ListArchived(context.Context) ([]RunnerRecord, error)
	Find(context.Context, string) (RunnerRecord, bool, error)
	SetDesiredState(context.Context, string, string) (RunnerRecord, bool, error)
	UpdateCapacity(context.Context, string, int) (RunnerRecord, bool, error)
	UpdateNATSEndpoint(context.Context, string, string) (RunnerRecord, bool, error)
	UpdateControlPlaneURL(context.Context, string, string) (RunnerRecord, bool, error)
	Delete(context.Context, string) (bool, error)
	Archive(context.Context, string) (bool, error)
	CreateEnrollment(context.Context, RunnerRecord, RunnerEnrollmentRecord) error
	ConsumeEnrollment(context.Context, string, time.Time) (RunnerRecord, error)
	ConsumeEnrollmentWithKey(context.Context, string, time.Time, string, []byte) (RunnerRecord, error)
	CreateSession(context.Context, string, string) error
	Heartbeat(context.Context, string, time.Time) error
	HeartbeatWithKey(context.Context, string, string, time.Time, string, []byte) error
	FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error)
	MarkStale(context.Context, time.Time) error
}

type RunnerStore struct{ pool database }

const DefaultRunnerCapacity = 10

var (
	ErrRunnerPoolInUse           = errors.New("runner pool is still in use")
	ErrRunnerPoolHasTaskVersions = errors.New("runner pool is still referenced by task versions")
	ErrRunnerHasExecutionHistory = errors.New("runner is referenced by execution history")
)

func NewRunnerRepository(pool any) *RunnerStore {
	db, _ := databaseFrom(pool)
	return &RunnerStore{pool: db}
}

func (s *RunnerStore) EnsurePool(ctx context.Context, id, name string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO runner_pools (id, name, description) VALUES ($1, $2, CASE WHEN $1 = 'default' THEN 'Default Runner Pool' ELSE '' END) ON CONFLICT (id) DO UPDATE SET description = CASE WHEN runner_pools.id = 'default' AND runner_pools.description = '' THEN EXCLUDED.description ELSE runner_pools.description END`, id, name)
	return err
}

func (s *RunnerStore) ListPools(ctx context.Context) ([]RunnerPoolRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, description, enabled, is_deleted FROM runner_pools WHERE NOT is_deleted ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RunnerPoolRecord{}
	for rows.Next() {
		var item RunnerPoolRecord
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Enabled, &item.IsDeleted); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *RunnerStore) FindPool(ctx context.Context, id string) (RunnerPoolRecord, bool, error) {
	var item RunnerPoolRecord
	err := s.pool.QueryRow(ctx, `SELECT id, name, description, enabled, is_deleted FROM runner_pools WHERE id = $1 AND NOT is_deleted`, id).Scan(&item.ID, &item.Name, &item.Description, &item.Enabled, &item.IsDeleted)
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
	result, err := s.pool.Exec(ctx, `UPDATE runner_pools SET name = $2, description = $3, enabled = $4, updated_at = now() WHERE id = $1 AND NOT is_deleted`, item.ID, item.Name, item.Description, item.Enabled)
	if err != nil || result.RowsAffected() == 0 {
		return RunnerPoolRecord{}, result.RowsAffected() > 0, err
	}
	return s.FindPool(ctx, item.ID)
}

func (s *RunnerStore) DeletePool(ctx context.Context, id string) error {
	var active bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM runners WHERE pool_id = $1 AND NOT is_archived AND NOT is_deleted)`, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrRunnerPoolInUse
	}
	var activeTasks bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM task_versions v JOIN tasks t ON t.id = v.task_id WHERE v.runner_pool_id = $1 AND NOT t.is_deleted)`, id).Scan(&activeTasks); err != nil {
		return err
	}
	if activeTasks {
		return ErrRunnerPoolHasTaskVersions
	}
	result, err := s.pool.Exec(ctx, `UPDATE runner_pools SET is_deleted = true, enabled = false, updated_at = now() WHERE id = $1 AND NOT is_deleted`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("runner pool not found")
	}
	return nil
}

const runnerQuery = `WITH latest_metrics AS (SELECT rm.runner_id, rm.sampled_at, rm.cpu_percent, rm.memory_percent, rm.memory_used_bytes, rm.memory_total_bytes, ROW_NUMBER() OVER (PARTITION BY rm.runner_id ORDER BY rm.sampled_at DESC) AS row_number FROM runner_metrics rm) SELECT r.id, r.name, p.id, p.name, r.desired_state, r.observed_state, r.nats_endpoint, r.control_plane_url, r.capacity, COALESCE((SELECT current_capacity FROM runner_sessions WHERE runner_id = r.id AND disconnected_at IS NULL ORDER BY last_heartbeat_at DESC LIMIT 1), 0), r.active_count, r.last_seen_at, COALESCE(r.capabilities->>'platform', ''), COALESCE(r.capabilities->>'architecture', ''), r.is_archived, r.is_deleted, m.sampled_at, m.cpu_percent, m.memory_percent, m.memory_used_bytes, m.memory_total_bytes FROM runners r LEFT JOIN runner_pools p ON p.id = r.pool_id LEFT JOIN latest_metrics m ON m.runner_id = r.id AND m.row_number = 1`

func (s *RunnerStore) List(ctx context.Context) ([]RunnerRecord, error) {
	rows, err := s.pool.Query(ctx, runnerQuery+` WHERE NOT r.is_archived AND NOT r.is_deleted ORDER BY r.id`)
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

func (s *RunnerStore) ListArchived(ctx context.Context) ([]RunnerRecord, error) {
	rows, err := s.pool.Query(ctx, runnerQuery+` WHERE r.is_archived OR r.is_deleted ORDER BY r.id`)
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
	var poolID, pool sql.NullString
	var sampledAt *time.Time
	var cpuPercent, memoryPercent sql.NullFloat64
	var memoryUsedBytes, memoryTotalBytes sql.NullInt64
	if err := row.Scan(&item.ID, &item.Name, &poolID, &pool, &item.DesiredState, &item.ObservedState, &item.NATSEndpoint, &item.ControlPlaneURL, &item.Capacity, &item.CurrentCapacity, &item.ActiveCount, &item.HeartbeatAt, &item.Platform, &item.Architecture, &item.IsArchived, &item.IsDeleted, &sampledAt, &cpuPercent, &memoryPercent, &memoryUsedBytes, &memoryTotalBytes); err != nil {
		return RunnerRecord{}, err
	}
	item.PoolID, item.Pool = poolID.String, pool.String
	if sampledAt != nil && cpuPercent.Valid && memoryPercent.Valid && memoryUsedBytes.Valid && memoryTotalBytes.Valid {
		item.CurrentMetrics = &RunnerMetricsRecord{SampledAt: sampledAt.UTC(), CPUPercent: cpuPercent.Float64, MemoryPercent: memoryPercent.Float64, MemoryUsedBytes: memoryUsedBytes.Int64, MemoryTotalBytes: memoryTotalBytes.Int64}
	}
	return item, nil
}

func (s *RunnerStore) SetDesiredState(ctx context.Context, id, state string) (RunnerRecord, bool, error) {
	query := `UPDATE runners SET desired_state = $2, updated_at = now() WHERE id = $1`
	if state == "REVOKED" {
		query = `UPDATE runners SET desired_state = 'DISABLED', observed_state = 'REVOKED', updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted`
	} else if state == "ENABLED" {
		query = `UPDATE runners SET desired_state = $2, observed_state = 'OFFLINE', updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted`
	} else {
		query += ` AND NOT is_archived AND NOT is_deleted`
	}
	var result rowsAffecter
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

func (s *RunnerStore) UpdateCapacity(ctx context.Context, id string, capacity int) (RunnerRecord, bool, error) {
	if capacity < 1 {
		return RunnerRecord{}, false, errors.New("runner capacity must be at least 1")
	}
	result, err := s.pool.Exec(ctx, `UPDATE runners SET capacity = $2, updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted`, id, capacity)
	if err != nil || result.RowsAffected() == 0 {
		return RunnerRecord{}, result.RowsAffected() > 0, err
	}
	return s.Find(ctx, id)
}

func (s *RunnerStore) UpdateNATSEndpoint(ctx context.Context, id, endpoint string) (RunnerRecord, bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE runners SET nats_endpoint = $2, updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted`, id, strings.TrimSpace(endpoint))
	if err != nil || result.RowsAffected() == 0 {
		return RunnerRecord{}, result.RowsAffected() > 0, err
	}
	return s.Find(ctx, id)
}

func (s *RunnerStore) UpdateControlPlaneURL(ctx context.Context, id, endpoint string) (RunnerRecord, bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE runners SET control_plane_url = $2, updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted`, id, strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	if err != nil || result.RowsAffected() == 0 {
		return RunnerRecord{}, result.RowsAffected() > 0, err
	}
	return s.Find(ctx, id)
}

func (s *RunnerStore) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM runners WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return false, ErrRunnerHasExecutionHistory
		}
	}
	return result.RowsAffected() > 0, err
}

func (s *RunnerStore) Archive(ctx context.Context, id string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var archived bool
	if err := tx.QueryRow(ctx, `SELECT is_archived OR is_deleted FROM runners WHERE id = $1 FOR UPDATE`, id).Scan(&archived); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if archived {
		return true, tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `UPDATE runners SET is_archived = true, is_deleted = true, desired_state = 'DISABLED', observed_state = 'OFFLINE', updated_at = now() WHERE id = $1`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runner_sessions SET disconnected_at = COALESCE(disconnected_at, now()) WHERE runner_id = $1 AND disconnected_at IS NULL`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runner_keys SET revoked_at = COALESCE(revoked_at, now()) WHERE runner_id = $1 AND revoked_at IS NULL`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM runner_enrollments WHERE runner_id = $1 AND used_at IS NULL`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET state = CASE WHEN state IN ('WAITING','RETRY_WAIT') THEN 'CANCELLED' ELSE 'CANCELLING' END, cancellation_requested_at = COALESCE(cancellation_requested_at, now()), cancellation_reason = 'runner archived', completed_at = CASE WHEN state IN ('WAITING','RETRY_WAIT') THEN now() ELSE completed_at END, state_version = state_version + 1, updated_at = now() WHERE id IN (SELECT run_id FROM execution_attempts WHERE runner_id = $1) AND state IN ('WAITING','RUNNING','RETRY_WAIT','CANCELLING')`, id); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
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
	if _, err := tx.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, observed_state, capacity, nats_endpoint, control_plane_url, capabilities) VALUES ($1, (SELECT id FROM runner_pools WHERE (id = $2 OR name = $2) AND NOT is_deleted), $3, 'ENABLED', 'PENDING', CASE WHEN $4 > 0 THEN $4 ELSE 10 END, $5, $6, $7::jsonb) ON CONFLICT (id) DO UPDATE SET capacity = CASE WHEN $4 > 0 THEN EXCLUDED.capacity ELSE runners.capacity END, nats_endpoint = CASE WHEN EXCLUDED.nats_endpoint <> '' THEN EXCLUDED.nats_endpoint ELSE runners.nats_endpoint END, control_plane_url = CASE WHEN EXCLUDED.control_plane_url <> '' THEN EXCLUDED.control_plane_url ELSE runners.control_plane_url END`, runner.ID, poolID, runner.Name, runner.Capacity, strings.TrimSpace(runner.NATSEndpoint), strings.TrimSpace(runner.ControlPlaneURL), artifact); err != nil {
		return err
	}
	var lockedRunnerID string
	var archived bool
	if err := tx.QueryRow(ctx, `SELECT id, is_archived OR is_deleted FROM runners WHERE id = $1 FOR UPDATE`, runner.ID).Scan(&lockedRunnerID, &archived); err != nil {
		return err
	}
	if archived {
		return errors.New("runner is archived")
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
	if _, err := tx.Exec(ctx, `UPDATE runners SET observed_state = 'PENDING', last_seen_at = NULL, updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted`, runnerID); err != nil {
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
	_, err := s.pool.Exec(ctx, `UPDATE runners SET observed_state = 'ONLINE', last_seen_at = $2, updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted AND observed_state <> 'REVOKED' AND (last_seen_at IS NULL OR last_seen_at < $2)`, runnerID, now)
	return err
}

func (s *RunnerStore) HeartbeatWithKey(ctx context.Context, runnerID, bootID string, now time.Time, keyID string, publicKey []byte) error {
	return s.HeartbeatWithKeyAndCapacity(ctx, runnerID, bootID, now, 0, keyID, publicKey)
}

func (s *RunnerStore) HeartbeatWithKeyAndCapacity(ctx context.Context, runnerID, bootID string, now time.Time, capacity int, keyID string, publicKey []byte) error {
	return s.heartbeatWithKeyAndCapacity(ctx, heartbeatInput{runnerID: runnerID, bootID: bootID, now: now, capacity: capacity, keyID: keyID, publicKey: publicKey})
}

func (s *RunnerStore) HeartbeatWithKeyAndCapacityAndMetrics(ctx context.Context, runnerID, bootID string, now time.Time, capacity int, sample RunnerMetricsSample, keyID string, publicKey []byte) error { // NOSONAR: preserve the exported repository interface used by control-plane heartbeat callers.
	if err := sample.validate(); err != nil {
		return err
	}
	return s.heartbeatWithKeyAndCapacity(ctx, heartbeatInput{runnerID: runnerID, bootID: bootID, now: now, capacity: capacity, sample: &sample, keyID: keyID, publicKey: publicKey})
}

type heartbeatInput struct {
	runnerID, bootID string
	now              time.Time
	capacity         int
	sample           *RunnerMetricsSample
	keyID            string
	publicKey        []byte
}

func (s *RunnerStore) heartbeatWithKeyAndCapacity(ctx context.Context, input heartbeatInput) error {
	if input.runnerID == "" || input.bootID == "" || input.keyID == "" || len(input.publicKey) != ed25519.PublicKeySize {
		return errors.New("runner heartbeat key is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	if err := tx.QueryRow(ctx, `SELECT NOT is_archived AND NOT is_deleted FROM runners WHERE id = $1 FOR UPDATE`, input.runnerID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("runner not found")
		}
		return err
	}
	if !active {
		return errors.New("runner is archived")
	}
	if _, err := tx.Exec(ctx, `UPDATE runner_sessions SET disconnected_at = $2 WHERE runner_id = $1 AND disconnected_at IS NULL AND boot_id <> $3`, input.runnerID, input.now, input.bootID); err != nil {
		return err
	}
	if input.capacity > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id, last_heartbeat_at, current_capacity) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (runner_id, boot_id) DO UPDATE SET last_heartbeat_at = EXCLUDED.last_heartbeat_at, disconnected_at = NULL, current_capacity = EXCLUDED.current_capacity`, input.runnerID+"/"+input.bootID, input.runnerID, input.bootID, input.now, input.capacity); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id, last_heartbeat_at) VALUES ($1, $2, $3, $4) ON CONFLICT (runner_id, boot_id) DO UPDATE SET last_heartbeat_at = EXCLUDED.last_heartbeat_at, disconnected_at = NULL`, input.runnerID+"/"+input.bootID, input.runnerID, input.bootID, input.now); err != nil {
		return err
	}
	var boundRunner string
	var boundKey []byte
	if err := tx.QueryRow(ctx, `SELECT runner_id, public_key FROM runner_keys WHERE key_id = $1 AND revoked_at IS NULL`, input.keyID).Scan(&boundRunner, &boundKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("runner heartbeat key is not enrolled")
		}
		return err
	}
	if boundRunner != input.runnerID || !ed25519.PublicKey(boundKey).Equal(ed25519.PublicKey(input.publicKey)) {
		return errors.New("runner heartbeat key does not match enrollment")
	}
	if _, err := tx.Exec(ctx, `UPDATE runners SET observed_state = 'ONLINE', last_seen_at = $2, updated_at = now() WHERE id = $1 AND NOT is_archived AND NOT is_deleted AND observed_state <> 'REVOKED'`, input.runnerID, input.now); err != nil {
		return err
	}
	if input.sample != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO runner_metrics (runner_id, sampled_at, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (runner_id, sampled_at) DO NOTHING`, input.runnerID, input.now, input.sample.CPUPercent, input.sample.MemoryPercent, input.sample.MemoryUsedBytes, input.sample.MemoryTotalBytes); err != nil {
			return err
		}
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
