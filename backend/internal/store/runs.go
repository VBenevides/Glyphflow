package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunDefinition struct {
	ID, TaskID, TriggeredBy, TriggerType, IdempotencyKey string
	ScheduledFor                                         time.Time
}

type RunRecord struct {
	ID, TaskID, TaskName, State, TriggerType string
	Runner                                   string
	Attempt                                  int
	ExitCode                                 *int
	ExitCodeMeaning                          string
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

func NewRunID() (string, error) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "run-" + hex.EncodeToString(raw), nil
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
	Pool                                    string
	RunnerID, RunnerSessionID, LeaseToken   string
	Command                                 []string
	WorkingDirectory, ExecutionSpecDigest   string
	Environment                             map[string]string
	SecretRefs                              []string
	PlacementSelectors                      map[string]any
	Resources                               map[string]string
	AttemptNumber                           int
	TimeoutSeconds, MaxOutputBytes          int
	FencingToken                            int64
	LeaseNotAfter                           time.Time
}

type DispatchOutboxRecord struct {
	MessageID, Subject string
	Envelope           []byte
}

type CancellationCandidate struct {
	RunID, TaskID, AttemptID, RunnerID, RunnerSessionID, LeaseToken string
	AttemptNumber                                                   int
	FencingToken                                                    int64
	LeaseNotAfter                                                   time.Time
	Reason                                                          string
}

type RunEventInput struct {
	EventID, OrderID, RunID, TaskID, RunnerID, RunnerSessionID, LeaseToken string
	EventType, EventChannel, Subject, Error, Result                        string
	ExitCode                                                               *int
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

// CancellationRepository is implemented by durable stores that can enqueue a
// signed, attempt-specific cancel order. It is optional for API fakes.
type CancellationRepository interface {
	RequestCancellation(context.Context, string, string) (RunRecord, bool, error)
}

type RetryRepository interface {
	Retry(context.Context, string, string) (RunRecord, bool, error)
}

type RunStore struct{ pool *pgxpool.Pool }

const (
	insertTaskRunSQL        = "INSERT INTO runs"
	insertResourceLeaseSQL  = "INSERT INTO resource_leases"
	insertDispatchOutboxSQL = "INSERT INTO dispatch_outbox (order_bytes)"
)

func NewRunRepository(pool *pgxpool.Pool) *RunStore { return &RunStore{pool: pool} }

const runQuery = `SELECT r.id, r.task_id, t.name, r.state, r.trigger_type, r.scheduled_for, COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = r.id), 1), COALESCE(latest.runner_id, ''), latest.exit_code, COALESCE(ec.meaning, '') FROM runs r JOIN tasks t ON t.id = r.task_id LEFT JOIN LATERAL (SELECT runner_id, exit_code FROM execution_attempts WHERE run_id = r.id ORDER BY attempt_number DESC LIMIT 1) latest ON true LEFT JOIN exit_code ec ON ec.code = latest.exit_code`

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
	if err := row.Scan(&item.ID, &item.TaskID, &item.TaskName, &item.State, &item.TriggerType, &item.ScheduledFor, &item.Attempt, &item.Runner, &item.ExitCode, &item.ExitCodeMeaning); err != nil {
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
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM tasks WHERE id = $1 AND NOT is_deleted`, definition.TaskID).Scan(&taskVersionID); err != nil {
		return RunRecord{}, err
	}
	variables, err := loadGlobalVariables(ctx, tx)
	if err != nil {
		return RunRecord{}, err
	}
	resolvedGlobals, err := json.Marshal(variables)
	if err != nil {
		return RunRecord{}, err
	}
	var triggeredBy any
	if strings.TrimSpace(definition.TriggeredBy) != "" {
		triggeredBy = definition.TriggeredBy
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runs (id, task_id, task_version_id, triggered_by, trigger_type, scheduled_for, resolved_global_variables, state, idempotency_key) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, 'WAITING', $8)`, definition.ID, definition.TaskID, taskVersionID, triggeredBy, definition.TriggerType, definition.ScheduledFor, resolvedGlobals, definition.IdempotencyKey); err != nil {
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
	var command, environment, secrets, selectors, resources, resolvedGlobals []byte
	var timeout, maxOutput, attempt int
	err = tx.QueryRow(ctx, `SELECT r.id, r.task_id, r.task_version_id, tv.command, COALESCE(NULLIF(tv.working_directory, ''), '.'), tv.timeout_seconds, tv.max_output_bytes, tv.execution_spec_digest, COALESCE(r.resolved_global_variables, '{}'::jsonb), COALESCE(tv.environment, '{}'::jsonb), COALESCE(tv.secret_references, '{}'::jsonb), COALESCE(tv.placement_selectors, '{}'::jsonb), COALESCE((SELECT jsonb_agg(resource_id) FROM task_resource_requirements WHERE task_version_id = tv.id), '[]'::jsonb), rp.name, rs.id, rs.runner_id, COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = r.id), 0) + 1 FROM runs r JOIN task_versions tv ON tv.id = r.task_version_id JOIN runners rr ON rr.pool_id = tv.runner_pool_id AND NOT rr.is_archived AND NOT rr.is_deleted AND rr.desired_state = 'ENABLED' AND rr.active_count < rr.capacity AND (tv.pinned_runner_id IS NULL OR tv.pinned_runner_id = rr.id) JOIN runner_pools rp ON rp.id = rr.pool_id JOIN runner_sessions rs ON rs.runner_id = rr.id AND rs.disconnected_at IS NULL AND rs.last_heartbeat_at >= now() - interval '30 seconds' WHERE (r.state = 'WAITING' OR (r.state = 'RETRY_WAIT' AND (r.retry_not_before IS NULL OR r.retry_not_before <= now()))) AND r.scheduled_for <= now() AND (r.state = 'WAITING' OR COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = r.id), 0) < tv.max_attempts) AND rr.capabilities @> COALESCE(tv.placement_selectors, '{}'::jsonb) AND NOT EXISTS (SELECT 1 FROM task_resource_requirements req JOIN resources resource ON resource.id = req.resource_id LEFT JOIN resource_leases lease ON lease.resource_id = req.resource_id AND lease.state = 'ACTIVE' AND lease.expires_at > now() WHERE req.task_version_id = tv.id AND (NOT resource.enabled OR lease.id IS NOT NULL)) ORDER BY r.created_at, r.id FOR UPDATE OF r, rr, rs SKIP LOCKED LIMIT 1`).Scan(&candidate.RunID, &candidate.TaskID, &candidate.TaskVersionID, &command, &candidate.WorkingDirectory, &timeout, &maxOutput, &candidate.ExecutionSpecDigest, &resolvedGlobals, &environment, &secrets, &selectors, &resources, &candidate.Pool, &candidate.RunnerSessionID, &candidate.RunnerID, &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DispatchCandidate{}, false, nil
	}
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	if err := json.Unmarshal(command, &candidate.Command); err != nil {
		return DispatchCandidate{}, false, err
	}
	candidate.Environment, err = decodeEnvironment(environment)
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	if err := json.Unmarshal(secrets, &candidate.SecretRefs); err != nil {
		// Secret references are historically stored as a JSON object. Keep the
		// values opaque and send only reference names to the worker.
		var secretMap map[string]any
		if json.Unmarshal(secrets, &secretMap) != nil {
			return DispatchCandidate{}, false, err
		}
		for key := range secretMap {
			candidate.SecretRefs = append(candidate.SecretRefs, key)
		}
		sort.Strings(candidate.SecretRefs)
	}
	if err := json.Unmarshal(selectors, &candidate.PlacementSelectors); err != nil {
		return DispatchCandidate{}, false, err
	}
	var resourceIDs []string
	if err := json.Unmarshal(resources, &resourceIDs); err != nil {
		return DispatchCandidate{}, false, err
	}
	candidate.Resources = make(map[string]string, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		candidate.Resources[resourceID] = resourceID
	}
	globals := map[string]string{}
	if err := json.Unmarshal(resolvedGlobals, &globals); err != nil {
		return DispatchCandidate{}, false, err
	}
	for index, argument := range candidate.Command {
		candidate.Command[index], err = platform.ResolveGlobalVariables(argument, globals)
		if err != nil {
			return DispatchCandidate{}, false, err
		}
	}
	candidate.WorkingDirectory, err = platform.ResolveGlobalVariables(candidate.WorkingDirectory, globals)
	if err != nil {
		return DispatchCandidate{}, false, err
	}
	for name, value := range candidate.Environment {
		candidate.Environment[name], err = platform.ResolveGlobalVariables(value, globals)
		if err != nil {
			return DispatchCandidate{}, false, err
		}
	}
	candidate.ExecutionSpecDigest, err = resolvedExecutionDigest(candidate)
	if err != nil {
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
	for resourceID := range candidate.Resources {
		if _, err := tx.Exec(ctx, `UPDATE resource_leases SET state = 'EXPIRED', released_at = now() WHERE resource_id = $1 AND state = 'ACTIVE' AND expires_at <= now()`, resourceID); err != nil {
			return DispatchCandidate{}, false, err
		}
		var fencingToken int64
		if err := tx.QueryRow(ctx, `UPDATE resources SET next_fencing_token = next_fencing_token + 1, updated_at = now() WHERE id = $1 AND enabled RETURNING next_fencing_token`, resourceID).Scan(&fencingToken); err != nil {
			return DispatchCandidate{}, false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO resource_leases (id, resource_id, execution_attempt_id, state, lease_token, fencing_token, acquired_at, expires_at) VALUES ($1, $2, $3, 'ACTIVE', $4, $5, now(), $6)`, randomLeaseID(), resourceID, candidate.AttemptID, randomLeaseID(), fencingToken, candidate.LeaseNotAfter); err != nil {
			return DispatchCandidate{}, false, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET state = 'RUNNING', state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state IN ('WAITING', 'RETRY_WAIT')`, candidate.RunID); err != nil {
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

// ReconcileTimedOutDispatches marks attempts that outlived their task timeout
// plus a grace period as UNKNOWN when no terminal runner event was received.
func (s *RunStore) ReconcileTimedOutDispatches(ctx context.Context, now time.Time) error {
	if now.IsZero() {
		return errors.New("dispatch timeout reconciliation time is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT a.id, a.run_id, a.runner_id, a.last_applied_state_sequence FROM runs r JOIN execution_attempts a ON a.run_id = r.id JOIN task_versions tv ON tv.id = r.task_version_id WHERE r.state = 'RUNNING' AND a.state IN ('DISPATCHED','ACCEPTED','RUNNING') AND a.dispatched_at IS NOT NULL AND a.dispatched_at + (tv.timeout_seconds * interval '1 second') + interval '10 minutes' <= $1 ORDER BY a.dispatched_at, a.id FOR UPDATE OF r, a SKIP LOCKED LIMIT 100`, now)
	if err != nil {
		return err
	}
	type timedOutDispatch struct {
		attemptID, runID, runnerID string
		lastSequence               int64
	}
	candidates := make([]timedOutDispatch, 0, 100)
	for rows.Next() {
		var candidate timedOutDispatch
		if err := rows.Scan(&candidate.attemptID, &candidate.runID, &candidate.runnerID, &candidate.lastSequence); err != nil {
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, candidate := range candidates {
		sequence := candidate.lastSequence + 1
		if sequence < 3 {
			sequence = 3
		}
		reportedAt := now.UTC()
		payload := []byte(`{"result":"","error":"execution exceeded its timeout and could not be confirmed"}`)
		if _, err := tx.Exec(ctx, `INSERT INTO run_events (event_id, execution_attempt_id, state_sequence, event_kind, reported_at, payload) VALUES ($1, $2, $3, 'unknown', $4, $5::jsonb)`, "dispatch-timeout:"+candidate.attemptID, candidate.attemptID, sequence, reportedAt, payload); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE execution_attempts SET state = 'UNKNOWN', finished_at = COALESCE(finished_at, $2), termination_reason = 'execution timeout could not be confirmed', last_applied_state_sequence = $3, state_version = state_version + 1, updated_at = now() WHERE id = $1`, candidate.attemptID, reportedAt, sequence); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE runs SET state = 'UNKNOWN', retry_not_before = NULL, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state = 'RUNNING'`, candidate.runID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE runners SET active_count = GREATEST(active_count - 1, 0), updated_at = now() WHERE id = $1 AND active_count > 0`, candidate.runnerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE resource_leases SET state = 'RELEASED', released_at = now() WHERE execution_attempt_id = $1 AND state = 'ACTIVE'`, candidate.attemptID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE dispatch_outbox SET state = 'FAILED', last_error = 'execution timeout could not be confirmed' WHERE execution_attempt_id = $1 AND message_type = 'execute' AND state = 'PENDING'`, candidate.attemptID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func decodeEnvironment(raw []byte) (map[string]string, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	environment := make(map[string]string, len(values))
	for name, value := range values {
		var text string
		if json.Unmarshal(value, &text) == nil {
			environment[name] = text
			continue
		}
		environment[name] = string(value)
	}
	return environment, nil
}

func resolvedExecutionDigest(candidate DispatchCandidate) (string, error) {
	canonical, err := json.Marshal(struct {
		TaskVersion, WorkingDirectory string
		Command                       []string
		Environment                   map[string]string
		SecretRefs                    []string
		PlacementSelectors            map[string]any
		Resources                     map[string]string
		TimeoutSeconds, MaxOutput     int
	}{candidate.TaskVersionID, candidate.WorkingDirectory, candidate.Command, candidate.Environment, candidate.SecretRefs, candidate.PlacementSelectors, candidate.Resources, candidate.TimeoutSeconds, candidate.MaxOutputBytes})
	if err != nil {
		return "", err
	}
	return sha256Hex(canonical), nil
}

func loadGlobalVariables(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT name, value FROM global_variables`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variables := map[string]string{}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		variables[name] = value
	}
	return variables, rows.Err()
}

func (s *RunStore) ClaimCancelling(ctx context.Context, build func(CancellationCandidate) ([]byte, error)) (CancellationCandidate, bool, error) {
	if build == nil {
		return CancellationCandidate{}, false, errors.New("cancellation order builder is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CancellationCandidate{}, false, err
	}
	defer tx.Rollback(ctx)
	var candidate CancellationCandidate
	err = tx.QueryRow(ctx, `SELECT r.id, r.task_id, a.id, a.runner_id, a.runner_session_id, a.lease_token, a.attempt_number, a.fencing_token, a.lease_not_after, COALESCE(r.cancellation_reason, '') FROM runs r JOIN execution_attempts a ON a.run_id = r.id WHERE r.state = 'CANCELLING' AND a.state IN ('DISPATCHED','ACCEPTED','RUNNING') AND a.cancel_requested_at IS NULL ORDER BY r.updated_at, r.id FOR UPDATE OF r, a SKIP LOCKED LIMIT 1`).Scan(&candidate.RunID, &candidate.TaskID, &candidate.AttemptID, &candidate.RunnerID, &candidate.RunnerSessionID, &candidate.LeaseToken, &candidate.AttemptNumber, &candidate.FencingToken, &candidate.LeaseNotAfter, &candidate.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return CancellationCandidate{}, false, nil
	}
	if err != nil {
		return CancellationCandidate{}, false, err
	}
	envelope, err := build(candidate)
	if err != nil {
		return CancellationCandidate{}, false, err
	}
	if len(envelope) == 0 {
		return CancellationCandidate{}, false, errors.New("cancellation order is empty")
	}
	if _, err := tx.Exec(ctx, `UPDATE execution_attempts SET cancel_requested_at = now(), updated_at = now() WHERE id = $1`, candidate.AttemptID); err != nil {
		return CancellationCandidate{}, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dispatch_outbox (message_id, execution_attempt_id, message_type, subject, envelope, state) VALUES ($1, $2, 'cancel', $3, $4, 'PENDING') ON CONFLICT (message_id) DO NOTHING`, "cancel:"+candidate.AttemptID, candidate.AttemptID, "glyphflow.orders."+candidate.RunnerID, envelope); err != nil {
		return CancellationCandidate{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CancellationCandidate{}, false, err
	}
	return candidate, true, nil
}

func (s *RunStore) ReconcileStaleCancellations(ctx context.Context, cutoff time.Time) error {
	if cutoff.IsZero() {
		return errors.New("cancellation reconciliation cutoff is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT a.id, a.run_id, a.runner_id, a.last_applied_state_sequence, COALESCE(r.cancellation_reason, '') FROM runs r JOIN execution_attempts a ON a.run_id = r.id WHERE r.state = 'CANCELLING' AND a.state IN ('DISPATCHED','ACCEPTED','RUNNING','CANCELLING') AND r.updated_at < $1 ORDER BY r.updated_at, r.id FOR UPDATE OF r, a SKIP LOCKED LIMIT 100`, cutoff)
	if err != nil {
		return err
	}
	type staleCancellation struct {
		attemptID, runID, runnerID string
		lastSequence               int64
		reason                     string
	}
	candidates := make([]staleCancellation, 0, 100)
	for rows.Next() {
		var candidate staleCancellation
		if err := rows.Scan(&candidate.attemptID, &candidate.runID, &candidate.runnerID, &candidate.lastSequence, &candidate.reason); err != nil {
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	reportedAt := time.Now().UTC()
	for _, candidate := range candidates {
		sequence := candidate.lastSequence + 1
		if sequence < 3 {
			sequence = 3
		}
		cancelled := candidate.reason == "runner archived"
		payload := []byte(`{"result":"","error":"cancellation could not be confirmed before timeout"}`)
		state, termination, eventKind := "UNKNOWN", "cancellation could not be confirmed", "unknown"
		if cancelled {
			payload = []byte(`{"result":"","error":"runner archived"}`)
			state, termination, eventKind = "CANCELLED", "runner archived", "cancelled"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO run_events (event_id, execution_attempt_id, state_sequence, event_kind, reported_at, payload) VALUES ($1, $2, $3, $4, $5, $6::jsonb)`, "cancellation-timeout:"+candidate.attemptID, candidate.attemptID, sequence, eventKind, reportedAt, payload); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE execution_attempts SET state = $2, finished_at = COALESCE(finished_at, $3), termination_reason = $4, last_applied_state_sequence = $5, state_version = state_version + 1, updated_at = now() WHERE id = $1`, candidate.attemptID, state, reportedAt, termination, sequence); err != nil {
			return err
		}
		runState := "UNKNOWN"
		if cancelled {
			runState = "CANCELLED"
		}
		if _, err := tx.Exec(ctx, `UPDATE runs SET state = $2, retry_not_before = NULL, completed_at = CASE WHEN $2 = 'CANCELLED' THEN COALESCE(completed_at, $3) ELSE completed_at END, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state = 'CANCELLING'`, candidate.runID, runState, reportedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE runners SET active_count = GREATEST(active_count - 1, 0), updated_at = now() WHERE id = $1 AND active_count > 0`, candidate.runnerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE resource_leases SET state = 'RELEASED', released_at = now() WHERE execution_attempt_id = $1 AND state = 'ACTIVE'`, candidate.attemptID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *RunStore) RequestCancellation(ctx context.Context, id, reason string) (RunRecord, bool, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return RunRecord{}, false, errors.New("cancellation reason is required")
	}
	result, err := s.pool.Exec(ctx, `UPDATE runs SET state = CASE WHEN state IN ('WAITING','RETRY_WAIT') THEN 'CANCELLED' ELSE 'CANCELLING' END, cancellation_requested_at = COALESCE(cancellation_requested_at, now()), cancellation_reason = $2, completed_at = CASE WHEN state IN ('WAITING','RETRY_WAIT') THEN now() ELSE completed_at END, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state IN ('WAITING','RUNNING','RETRY_WAIT','CANCELLING')`, id, reason)
	if err != nil {
		return RunRecord{}, false, err
	}
	if result.RowsAffected() == 0 {
		return RunRecord{}, false, nil
	}
	item, found, err := s.Find(ctx, id)
	return item, found, err
}

func (s *RunStore) Retry(ctx context.Context, id, reason string) (RunRecord, bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE runs SET state = 'RETRY_WAIT', retry_not_before = now(), completed_at = NULL, cancellation_reason = NULLIF($2, ''), state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state IN ('FAILED','UNKNOWN') AND COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = runs.id), 0) < (SELECT max_attempts FROM task_versions WHERE id = runs.task_version_id)`, id, strings.TrimSpace(reason))
	if err != nil {
		return RunRecord{}, false, err
	}
	if result.RowsAffected() == 0 {
		return RunRecord{}, false, nil
	}
	item, found, err := s.Find(ctx, id)
	return item, found, err
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
	var attemptID, runnerID, sessionID, leaseToken, attemptState string
	var fencingToken int64
	var lastSequence int64
	if err := tx.QueryRow(ctx, `SELECT id, runner_id, runner_session_id, lease_token, fencing_token, state, last_applied_state_sequence FROM execution_attempts WHERE run_id = $1 AND attempt_number = $2 AND runner_id = $3 AND lease_token = $4 FOR UPDATE`, event.RunID, event.Attempt, event.RunnerID, event.LeaseToken).Scan(&attemptID, &runnerID, &sessionID, &leaseToken, &fencingToken, &attemptState, &lastSequence); err != nil {
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
	if event.EventType == "log_chunk" {
		if event.EventChannel != "stdout" && event.EventChannel != "stderr" {
			return errors.New("log chunk stream is invalid")
		}
		if event.Result == "" {
			return errors.New("log chunk is empty")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO execution_log_chunks (event_id, execution_attempt_id, stream, chunk_sequence, reported_at, payload, size_bytes, checksum) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`, event.EventID, attemptID, event.EventChannel, event.Sequence, event.ReportedAt, []byte(event.Result), len([]byte(event.Result)), sha256Hex([]byte(event.Result))); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if event.Sequence <= lastSequence {
		return tx.Commit(ctx)
	}
	if !legalAttemptTransition(attemptState, event.EventType) {
		return errors.New("run event transition is not allowed")
	}
	payloadValue := map[string]any{"result": event.Result, "error": event.Error}
	if event.ExitCode != nil {
		payloadValue["exit_code"] = *event.ExitCode
	}
	payload, _ := json.Marshal(payloadValue)
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
		_, err = tx.Exec(ctx, `UPDATE execution_attempts SET state = $2, finished_at = COALESCE(finished_at, $3), termination_reason = NULLIF($4, ''), exit_code = $5, result = $6::jsonb, updated_at = now() WHERE id = $1`, attemptID, state, event.ReportedAt, event.Error, event.ExitCode, payload)
		if err == nil {
			if event.EventType == "failed" || event.EventType == "timed_out" {
				var maxAttempts, initialBackoff, maxBackoff int
				var multiplier float64
				var exitCodes, reasons []byte
				if queryErr := tx.QueryRow(ctx, `SELECT max_attempts, initial_backoff_seconds, max_backoff_seconds, backoff_multiplier, retryable_exit_codes, retryable_termination_reasons FROM task_versions tv JOIN runs r ON r.task_version_id = tv.id WHERE r.id = $1`, event.RunID).Scan(&maxAttempts, &initialBackoff, &maxBackoff, &multiplier, &exitCodes, &reasons); queryErr != nil {
					err = queryErr
				} else if shouldRetry(event, int(maxAttempts), exitCodes, reasons) {
					if multiplier < 1 {
						multiplier = 2
					}
					seconds := float64(initialBackoff)
					for index := 1; index < int(event.Attempt); index++ {
						seconds *= multiplier
						if maxBackoff > 0 && seconds >= float64(maxBackoff) {
							seconds = float64(maxBackoff)
							break
						}
					}
					if maxBackoff > 0 && seconds > float64(maxBackoff) {
						seconds = float64(maxBackoff)
					}
					_, err = tx.Exec(ctx, `UPDATE runs SET state = 'RETRY_WAIT', retry_not_before = now() + ($2 * interval '1 second'), completed_at = NULL, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED')`, event.RunID, seconds)
				} else {
					_, err = tx.Exec(ctx, `UPDATE runs SET state = $2, completed_at = $3, retry_not_before = NULL, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED')`, event.RunID, state, event.ReportedAt)
				}
			} else {
				_, err = tx.Exec(ctx, `UPDATE runs SET state = $2, completed_at = $3, retry_not_before = NULL, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED')`, event.RunID, state, event.ReportedAt)
			}
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE runners SET active_count = GREATEST(active_count - 1, 0), updated_at = now() WHERE id = $1 AND active_count > 0`, runnerID)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE resource_leases SET state = 'RELEASED', released_at = now() WHERE execution_attempt_id = $1 AND state = 'ACTIVE'`, attemptID)
		}
	case "unknown":
		_, err = tx.Exec(ctx, `UPDATE execution_attempts SET state = 'UNKNOWN', finished_at = COALESCE(finished_at, $2), termination_reason = 'runner restart', updated_at = now() WHERE id = $1`, attemptID, event.ReportedAt)
		if err == nil {
			var policy string
			if queryErr := tx.QueryRow(ctx, `SELECT ambiguity_policy FROM task_versions tv JOIN runs r ON r.task_version_id = tv.id WHERE r.id = $1`, event.RunID).Scan(&policy); queryErr != nil {
				err = queryErr
			} else {
				state, resolveErr := platform.ResolveAmbiguous(platform.AmbiguityPolicy(policy))
				if resolveErr != nil {
					err = resolveErr
				} else if state == "retry_wait" {
					_, err = tx.Exec(ctx, `UPDATE runs SET state = 'RETRY_WAIT', retry_not_before = now() + interval '1 second', state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED','FAILED','CANCELLED')`, event.RunID)
				} else if state == "failed" {
					_, err = tx.Exec(ctx, `UPDATE runs SET state = 'FAILED', completed_at = $2, state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED','FAILED','CANCELLED')`, event.RunID, event.ReportedAt)
				} else {
					_, err = tx.Exec(ctx, `UPDATE runs SET state = 'UNKNOWN', state_version = state_version + 1, updated_at = now() WHERE id = $1 AND state NOT IN ('SUCCEEDED','FAILED','CANCELLED')`, event.RunID)
				}
			}
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE runners SET active_count = GREATEST(active_count - 1, 0), updated_at = now() WHERE id = $1 AND active_count > 0`, runnerID)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE resource_leases SET state = 'RELEASED', released_at = now() WHERE execution_attempt_id = $1 AND state = 'ACTIVE'`, attemptID)
		}
	default:
		return errors.New("unsupported run event type")
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE execution_attempts SET last_applied_state_sequence = $2, state_version = state_version + 1, updated_at = now() WHERE id = $1`, attemptID, event.Sequence); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func shouldRetry(event RunEventInput, maxAttempts int, exitCodes, reasons []byte) bool {
	if maxAttempts <= int(event.Attempt) {
		return false
	}
	var codes []int
	var retryReasons []string
	_ = json.Unmarshal(exitCodes, &codes)
	_ = json.Unmarshal(reasons, &retryReasons)
	if len(codes) > 0 {
		if event.ExitCode == nil {
			return false
		}
		matched := false
		for _, code := range codes {
			if *event.ExitCode == code {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(retryReasons) > 0 {
		matched := false
		for _, reason := range retryReasons {
			if reason == event.Error {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func legalAttemptTransition(state, eventType string) bool {
	switch eventType {
	case "accepted":
		return state == "DISPATCHED"
	case "started":
		return state == "ACCEPTED" || state == "DISPATCHED"
	case "heartbeat":
		return state == "ACCEPTED" || state == "RUNNING"
	case "completed", "failed", "timed_out", "cancelled":
		return state == "ACCEPTED" || state == "RUNNING" || state == "CANCELLING"
	case "unknown":
		return state == "DISPATCHED" || state == "ACCEPTED" || state == "RUNNING" || state == "CANCELLING"
	default:
		return false
	}
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
