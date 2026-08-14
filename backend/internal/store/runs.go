package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
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

type DispatchCandidate struct {
	RunID, TaskID, TaskVersionID, AttemptID string
	RunnerID, RunnerSessionID, LeaseToken   string
	Command                                 []string
	WorkingDirectory, ExecutionSpecDigest   string
	AttemptNumber                           int
	TimeoutSeconds, MaxOutputBytes          int
	FencingToken                            int64
	LeaseNotAfter                           time.Time
}

type DispatchOutboxRecord struct {
	MessageID, Subject string
	Envelope           []byte
}

type RunEventInput struct {
	EventID, OrderID, RunID, TaskID, RunnerID, RunnerSessionID, LeaseToken string
	EventType, Subject, Error, Result                                      string
	Attempt, Sequence, FencingToken                                        int64
	ReportedAt                                                             time.Time
	Envelope                                                               []byte
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

func (s *RunStore) ClaimWaiting(ctx context.Context, build func(DispatchCandidate) ([]byte, error)) (DispatchCandidate, bool, error) {
	if build == nil {
		return DispatchCandidate{}, false, errors.New("dispatch order builder is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	defer tx.Rollback(ctx)
	var candidate DispatchCandidate
	var command []byte
	var timeout, maxOutput, attempt int
	err = tx.QueryRow(ctx, `SELECT r.id, r.task_id, r.task_version_id, tv.command, COALESCE(NULLIF(tv.working_directory, ''), '.'), tv.timeout_seconds, tv.max_output_bytes, tv.execution_spec_digest, rs.id, rs.runner_id, COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = r.id), 0) + 1 FROM runs r JOIN task_versions tv ON tv.id = r.task_version_id JOIN runners rr ON rr.pool_id = tv.runner_pool_id AND rr.desired_state = 'ENABLED' AND rr.active_count < rr.capacity AND (tv.pinned_runner_id IS NULL OR tv.pinned_runner_id = rr.id) JOIN runner_sessions rs ON rs.runner_id = rr.id AND rs.disconnected_at IS NULL AND rs.last_heartbeat_at >= now() - interval '30 seconds' WHERE r.state = 'WAITING' AND r.scheduled_for <= now() ORDER BY r.created_at, r.id FOR UPDATE OF r, rr, rs SKIP LOCKED LIMIT 1`).Scan(&candidate.RunID, &candidate.TaskID, &candidate.TaskVersionID, &command, &candidate.WorkingDirectory, &timeout, &maxOutput, &candidate.ExecutionSpecDigest, &candidate.RunnerSessionID, &candidate.RunnerID, &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DispatchCandidate{}, false, nil
	}
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	if err := json.Unmarshal(command, &candidate.Command); err != nil {
		return DispatchCandidate{}, false, err
	}
	candidate.AttemptNumber = attempt
	candidate.AttemptID = candidate.RunID + "-attempt-" + strconv.Itoa(attempt)
	candidate.LeaseToken, err = randomToken()
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	candidate.FencingToken = time.Now().UTC().UnixNano()
	candidate.LeaseNotAfter = time.Now().UTC().Add(time.Duration(timeout+30) * time.Second)
	candidate.TimeoutSeconds = timeout
	candidate.MaxOutputBytes = maxOutput
	envelope, err := build(candidate)
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	if len(envelope) == 0 {
		return DispatchCandidate{}, false, errors.New("dispatch order is empty")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO execution_attempts (id, run_id, attempt_number, runner_id, runner_session_id, state, lease_token, fencing_token, lease_not_after, execution_spec_digest, dispatched_at) VALUES ($1, $2, $3, $4, $5, 'DISPATCHED', $6, $7, $8, $9, now())`, candidate.AttemptID, candidate.RunID, candidate.AttemptNumber, candidate.RunnerID, candidate.RunnerSessionID, candidate.LeaseToken, candidate.FencingToken, candidate.LeaseNotAfter, candidate.ExecutionSpecDigest); err != nil {
		return DispatchCandidate{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET state = 'RUNNING', state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state = 'WAITING'`, candidate.RunID); err != nil {
		return DispatchCandidate{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runners SET active_count = active_count + 1, updated_at = now() WHERE id = $1`, candidate.RunnerID); err != nil {
		return DispatchCandidate{}, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dispatch_outbox (message_id, execution_attempt_id, message_type, subject, envelope, state) VALUES ($1, $2, 'execute', $3, $4, 'PENDING')`, candidate.AttemptID, candidate.AttemptID, "glyphflow.orders."+candidate.RunnerID, envelope); err != nil {
		return DispatchCandidate{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DispatchCandidate{}, false, err
	}
	return candidate, true, nil
}

func (s *RunStore) PendingDispatch(ctx context.Context, limit int) ([]DispatchOutboxRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT message_id, subject, envelope FROM dispatch_outbox WHERE state = 'PENDING' AND available_at <= now() ORDER BY created_at, message_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DispatchOutboxRecord{}
	for rows.Next() {
		var item DispatchOutboxRecord
		if err := rows.Scan(&item.MessageID, &item.Subject, &item.Envelope); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *RunStore) MarkDispatchPublished(ctx context.Context, messageID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE dispatch_outbox SET state = 'PUBLISHED', published_at = now() WHERE message_id = $1 AND state = 'PENDING'`, messageID)
	return err
}

func (s *RunStore) RetryDispatch(ctx context.Context, messageID string, dispatchErr error) error {
	_, err := s.pool.Exec(ctx, `UPDATE dispatch_outbox SET publish_attempts = publish_attempts + 1, last_error = $2, available_at = now() + interval '1 second' WHERE message_id = $1 AND state = 'PENDING'`, messageID, dispatchErr.Error())
	return err
}

func (s *RunStore) ApplyRunEvent(ctx context.Context, event RunEventInput) error {
	if event.EventID == "" || event.RunID == "" || event.Attempt <= 0 || event.Sequence <= 0 || event.EventType == "" || len(event.Envelope) == 0 {
		return errors.New("run event is incomplete")
	}
	if event.ReportedAt.IsZero() {
		event.ReportedAt = time.Now().UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var attemptID, runnerID, sessionID, leaseToken string
	var fencingToken int64
	if err := tx.QueryRow(ctx, `SELECT id, runner_id, runner_session_id, lease_token, fencing_token FROM execution_attempts WHERE run_id = $1 AND attempt_number = $2 AND runner_id = $3 AND lease_token = $4`, event.RunID, event.Attempt, event.RunnerID, event.LeaseToken).Scan(&attemptID, &runnerID, &sessionID, &leaseToken, &fencingToken); err != nil {
		return err
	}
	if event.RunnerSessionID != "" && event.RunnerSessionID != sessionID {
		return errors.New("run event runner session does not match attempt")
	}
	if event.FencingToken != 0 && event.FencingToken != fencingToken {
		return errors.New("run event fencing token does not match attempt")
	}
	result, err := tx.Exec(ctx, `INSERT INTO event_inbox (event_id, execution_attempt_id, event_type, subject, envelope) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (event_id) DO NOTHING`, event.EventID, attemptID, event.EventType, event.Subject, event.Envelope)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	payload, _ := json.Marshal(map[string]any{"result": event.Result, "error": event.Error})
	if _, err := tx.Exec(ctx, `INSERT INTO run_events (event_id, execution_attempt_id, state_sequence, event_kind, reported_at, payload) VALUES ($1, $2, $3, $4, $5, $6::jsonb)`, event.EventID, attemptID, event.Sequence, event.EventType, event.ReportedAt, payload); err != nil {
		return err
	}
	if event.Result != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO execution_log_chunks (event_id, execution_attempt_id, stream, chunk_sequence, reported_at, payload, size_bytes, checksum) VALUES ($1, $2, 'stdout', $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`, event.EventID+":stdout", attemptID, event.Sequence, event.ReportedAt, []byte(event.Result), len([]byte(event.Result)), sha256Hex([]byte(event.Result))); err != nil {
			return err
		}
	}
	switch event.EventType {
	case "accepted":
		_, err = tx.Exec(ctx, `UPDATE execution_attempts SET state = 'ACCEPTED', accepted_at = COALESCE(accepted_at, $2), updated_at = now() WHERE id = $1`, attemptID, event.ReportedAt)
	case "started":
		_, err = tx.Exec(ctx, `UPDATE execution_attempts SET state = 'RUNNING', started_at = COALESCE(started_at, $2), updated_at = now() WHERE id = $1`, attemptID, event.ReportedAt)
	case "heartbeat":
		_, err = tx.Exec(ctx, `UPDATE execution_attempts SET last_heartbeat_at = $2, updated_at = now() WHERE id = $1`, attemptID, event.ReportedAt)
	case "completed", "failed", "timed_out", "cancelled":
		state := map[string]string{"completed": "SUCCEEDED", "failed": "FAILED", "timed_out": "FAILED", "cancelled": "CANCELLED"}[event.EventType]
		_, err = tx.Exec(ctx, `UPDATE execution_attempts SET state = $2, finished_at = COALESCE(finished_at, $3), termination_reason = NULLIF($4, ''), result = $5::jsonb, updated_at = now() WHERE id = $1`, attemptID, state, event.ReportedAt, event.Error, payload)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE runs SET state = $2, completed_at = $3, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED')`, event.RunID, state, event.ReportedAt)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE runners SET active_count = GREATEST(active_count - 1, 0), updated_at = now() WHERE id = $1 AND active_count > 0`, runnerID)
		}
	default:
		return errors.New("unsupported run event type")
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
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

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
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
