package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunDefinition struct {
	ID, TaskID, TriggeredBy, TriggerType, IdempotencyKey string
	ScheduledFor                                         time.Time
}

type RunRecord struct {
	ID, TaskID, TaskName, State, TriggerType string
	Attempt                                  int
	ScheduledFor                             time.Time
}

type RunLogChunkRecord struct {
	EventID, AttemptID string
	Sequence           int64
	Stream             string
	Text               string
	Payload            []byte
	ReportedAt         time.Time
	Checksum           string
}

type ExecutionAttemptDefinition struct {
	ID, RunID, RunnerID, RunnerSessionID, LeaseToken, ExecutionSpecDigest string
	AttemptNumber                                                         int
	FencingToken                                                          int64
	LeaseNotAfter                                                         time.Time
}

type RunEventRecord struct {
	EventID, AttemptID, EventKind string
	StateSequence                 int64
	ReportedAt                    time.Time
	Payload                       map[string]any
}

type RunRepository interface {
	List(context.Context) ([]RunRecord, error)
	Find(context.Context, string) (RunRecord, bool, error)
	Create(context.Context, RunDefinition) (RunRecord, error)
	Transition(context.Context, string, []string, string) (RunRecord, bool, error)
	CreateAttempt(context.Context, ExecutionAttemptDefinition) error
	AppendEvent(context.Context, RunEventRecord) error
	AppendLogChunk(context.Context, RunLogChunkRecord) error
	ListLogChunks(context.Context, string, string, int64) ([]RunLogChunkRecord, error)
}

type RunStore struct{ pool *pgxpool.Pool }

const (
	insertTaskRunSQL        = "INSERT INTO runs"
	insertResourceLeaseSQL  = "INSERT INTO resource_leases"
	insertDispatchOutboxSQL = "INSERT INTO dispatch_outbox (order_bytes)"
)

func NewRunRepository(pool *pgxpool.Pool) *RunStore { return &RunStore{pool: pool} }

const runQuery = `SELECT r.id, r.task_id, t.name, r.state, r.trigger_type, r.scheduled_for, COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = r.id), 1) FROM runs r JOIN tasks t ON t.id = r.task_id`

func (s *RunStore) List(ctx context.Context) ([]RunRecord, error) {
	rows, err := s.pool.Query(ctx, runQuery+` ORDER BY r.created_at DESC, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RunRecord{}
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *RunStore) Find(ctx context.Context, id string) (RunRecord, bool, error) {
	item, err := scanRun(s.pool.QueryRow(ctx, runQuery+` WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return RunRecord{}, false, nil
	}
	return item, err == nil, err
}

func scanRun(row interface{ Scan(...any) error }) (RunRecord, error) {
	var item RunRecord
	if err := row.Scan(&item.ID, &item.TaskID, &item.TaskName, &item.State, &item.TriggerType, &item.ScheduledFor, &item.Attempt); err != nil {
		return RunRecord{}, err
	}
	return item, nil
}

func (s *RunStore) Create(ctx context.Context, definition RunDefinition) (RunRecord, error) {
	if definition.TriggerType == "" {
		definition.TriggerType = "MANUAL"
	}
	if definition.ScheduledFor.IsZero() {
		definition.ScheduledFor = time.Now().UTC()
	}
	if definition.IdempotencyKey == "" {
		definition.IdempotencyKey = definition.ID
	}
	if definition.TriggerType != "MANUAL" && definition.TriggerType != "SCHEDULE" && definition.TriggerType != "RETRY" {
		return RunRecord{}, errors.New("invalid run trigger")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RunRecord{}, err
	}
	defer tx.Rollback(ctx)
	var taskVersionID string
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM tasks WHERE id = $1`, definition.TaskID).Scan(&taskVersionID); err != nil {
		return RunRecord{}, err
	}
	var triggeredBy any
	if strings.TrimSpace(definition.TriggeredBy) != "" {
		triggeredBy = definition.TriggeredBy
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runs (id, task_id, task_version_id, triggered_by, trigger_type, scheduled_for, state, idempotency_key) VALUES ($1, $2, $3, $4, $5, $6, 'WAITING', $7)`, definition.ID, definition.TaskID, taskVersionID, triggeredBy, definition.TriggerType, definition.ScheduledFor, definition.IdempotencyKey); err != nil {
		return RunRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RunRecord{}, err
	}
	item, found, err := s.Find(ctx, definition.ID)
	if err != nil {
		return RunRecord{}, err
	}
	if !found {
		return RunRecord{}, errors.New("run was not created")
	}
	return item, nil
}

func (s *RunStore) Transition(ctx context.Context, id string, from []string, to string) (RunRecord, bool, error) {
	if len(from) == 0 || to == "" {
		return RunRecord{}, false, errors.New("run transition is incomplete")
	}
	result, err := s.pool.Exec(ctx, `UPDATE runs SET state = $2, state_version = state_version + 1, updated_at = now(), completed_at = CASE WHEN $2 IN ('SUCCEEDED', 'FAILED', 'CANCELLED') THEN now() ELSE completed_at END WHERE id = $1 AND state = ANY($3::text[])`, id, to, from)
	if err != nil {
		return RunRecord{}, false, err
	}
	if result.RowsAffected() == 0 {
		return RunRecord{}, false, nil
	}
	item, found, err := s.Find(ctx, id)
	return item, found, err
}

func (s *RunStore) CreateAttempt(ctx context.Context, attempt ExecutionAttemptDefinition) error {
	if attempt.AttemptNumber <= 0 || attempt.FencingToken <= 0 || attempt.LeaseNotAfter.IsZero() {
		return errors.New("execution attempt is incomplete")
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO execution_attempts (id, run_id, attempt_number, runner_id, runner_session_id, state, lease_token, fencing_token, lease_not_after, execution_spec_digest) VALUES ($1, $2, $3, $4, $5, 'WAITING', $6, $7, $8, $9)`, attempt.ID, attempt.RunID, attempt.AttemptNumber, attempt.RunnerID, attempt.RunnerSessionID, attempt.LeaseToken, attempt.FencingToken, attempt.LeaseNotAfter, attempt.ExecutionSpecDigest)
	return err
}

func (s *RunStore) AppendEvent(ctx context.Context, event RunEventRecord) error {
	if event.ReportedAt.IsZero() {
		event.ReportedAt = time.Now().UTC()
	}
	if event.StateSequence <= 0 || event.EventID == "" || event.AttemptID == "" || event.EventKind == "" {
		return errors.New("run event is incomplete")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO run_events (event_id, execution_attempt_id, state_sequence, event_kind, reported_at, payload) VALUES ($1, $2, $3, $4, $5, $6::jsonb)`, event.EventID, event.AttemptID, event.StateSequence, event.EventKind, event.ReportedAt, payload)
	return err
}

func (s *RunStore) AppendLogChunk(ctx context.Context, chunk RunLogChunkRecord) error {
	if chunk.EventID == "" || chunk.AttemptID == "" || chunk.Sequence <= 0 || (chunk.Stream != "stdout" && chunk.Stream != "stderr") {
		return errors.New("log chunk is incomplete")
	}
	if chunk.ReportedAt.IsZero() {
		chunk.ReportedAt = time.Now().UTC()
	}
	if chunk.Checksum == "" {
		digest := sha256.Sum256(chunk.Payload)
		chunk.Checksum = hex.EncodeToString(digest[:])
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO execution_log_chunks (event_id, execution_attempt_id, stream, chunk_sequence, reported_at, payload, size_bytes, checksum) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, chunk.EventID, chunk.AttemptID, chunk.Stream, chunk.Sequence, chunk.ReportedAt, chunk.Payload, len(chunk.Payload), chunk.Checksum)
	return err
}

func (s *RunStore) ListLogChunks(ctx context.Context, runID, stream string, after int64) ([]RunLogChunkRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT l.chunk_sequence, l.stream, convert_from(l.payload, 'UTF8') FROM execution_log_chunks l JOIN execution_attempts a ON a.id = l.execution_attempt_id WHERE a.run_id = $1 AND l.stream = $2 AND l.chunk_sequence > $3 ORDER BY l.chunk_sequence`, runID, stream, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RunLogChunkRecord{}
	for rows.Next() {
		var item RunLogChunkRecord
		if err := rows.Scan(&item.Sequence, &item.Stream, &item.Text); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
