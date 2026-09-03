package store

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type coverageRowsAffected int64

func (r coverageRowsAffected) RowsAffected() int64 { return int64(r) }

type coverageDatabase struct {
	database
	result   rowsAffecter
	err      error
	execErr  error
	queryErr error
	tx       databaseTx
	beginErr error
	row      scanner
	rows     databaseRows
}

func (d coverageDatabase) Exec(context.Context, string, ...any) (rowsAffecter, error) {
	if d.result == nil {
		d.result = coverageRowsAffected(0)
	}
	if d.execErr != nil {
		return d.result, d.execErr
	}
	return d.result, d.err
}

func (d coverageDatabase) Begin(context.Context) (databaseTx, error) {
	return d.tx, d.beginErr
}

func (d coverageDatabase) Query(context.Context, string, ...any) (databaseRows, error) {
	if d.queryErr != nil {
		return d.rows, d.queryErr
	}
	return d.rows, d.err
}

func (d coverageDatabase) QueryRow(context.Context, string, ...any) scanner { return d.row }

type coverageTx struct {
	databaseTx
	result    rowsAffecter
	err       error
	execErr   error
	queryErr  error
	commitErr error
	row       scanner
	rows      databaseRows
}

func (tx coverageTx) Exec(context.Context, string, ...any) (rowsAffecter, error) {
	if tx.result == nil {
		tx.result = coverageRowsAffected(0)
	}
	if tx.execErr != nil {
		return tx.result, tx.execErr
	}
	return tx.result, tx.err
}

func (tx coverageTx) Commit(context.Context) error { return tx.commitErr }

func (tx coverageTx) Rollback(context.Context) error { return nil }

func (tx coverageTx) QueryRow(context.Context, string, ...any) scanner { return tx.row }

func (tx coverageTx) Query(context.Context, string, ...any) (databaseRows, error) {
	if tx.queryErr != nil {
		return tx.rows, tx.queryErr
	}
	return tx.rows, tx.err
}

type coverageExecStep struct {
	result rowsAffecter
	err    error
}

type coverageExecSequence struct {
	coverageTx
	steps []coverageExecStep
	index int
}

func (tx *coverageExecSequence) Exec(context.Context, string, ...any) (rowsAffecter, error) {
	if tx.index >= len(tx.steps) {
		return coverageRowsAffected(0), errors.New("unexpected coverage exec")
	}
	step := tx.steps[tx.index]
	tx.index++
	if step.result == nil {
		step.result = coverageRowsAffected(1)
	}
	return step.result, step.err
}

type coverageScanner func(...any) error

func (s coverageScanner) Scan(dest ...any) error { return s(dest...) }

type coverageScanSequence struct {
	scans []coverageScanner
	index int
}

func (s *coverageScanSequence) Scan(dest ...any) error {
	if s.index >= len(s.scans) {
		return errors.New("unexpected coverage scan")
	}
	scan := s.scans[s.index]
	s.index++
	return scan(dest...)
}

type coverageRows struct {
	rows  []func(...any) error
	index int
	err   error
}

func (r *coverageRows) Close() error { return nil }

func (r *coverageRows) Next() bool {
	return r.index < len(r.rows)
}

func (r *coverageRows) Scan(dest ...any) error {
	if !r.Next() {
		return errors.New("no coverage row")
	}
	err := r.rows[r.index](dest...)
	r.index++
	return err
}

func (r *coverageRows) Err() error { return r.err }

func TestStoreResultBranches(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name string
		rows int64
		err  error
		want error
	}{
		{name: "deleted", rows: 1},
		{name: "missing", rows: 0, want: errors.New("resource is missing or in use")},
		{name: "database error", err: errors.New("database unavailable"), want: errors.New("database unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &ResourceStore{pool: coverageDatabase{result: coverageRowsAffected(test.rows), err: test.err}}
			err := store.Delete(ctx, "resource")
			if (err == nil) != (test.want == nil) || err != nil && err.Error() != test.want.Error() {
				t.Fatalf("Delete error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestScheduleStoragePressureBranches(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name     string
		provider func(context.Context) (platform.StoragePressure, error)
		want     error
	}{
		{name: "unset"},
		{name: "provider error", provider: func(context.Context) (platform.StoragePressure, error) {
			return platform.StoragePressure{}, errors.New("pressure failed")
		}, want: errors.New("pressure failed")},
		{name: "unavailable", provider: func(context.Context) (platform.StoragePressure, error) {
			return platform.StoragePressure{State: platform.StorageUnavailable}, nil
		}, want: ErrStorageUnavailable},
		{name: "exhausted", provider: func(context.Context) (platform.StoragePressure, error) {
			return platform.StoragePressure{State: platform.StorageEmergency}, nil
		}, want: ErrStorageExhausted},
		{name: "normal", provider: func(context.Context) (platform.StoragePressure, error) {
			return platform.StoragePressure{State: platform.StorageNormal}, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &ScheduleStore{storagePressure: test.provider}
			if err := store.checkStoragePressure(ctx); (err == nil) != (test.want == nil) {
				t.Fatalf("pressure error = %v, want %v", err, test.want)
			}
		})
	}
	if _, changed, err := (&ScheduleStore{}).CreateDueRun(ctx, time.Now(), nil); err == nil || changed {
		t.Fatal("nil schedule next-fire function was accepted")
	}
}

func TestRetentionBranches(t *testing.T) {
	ctx := context.Background()
	if err := (&RetentionStore{}).SetLegalHold(ctx, "run", "id", true, "keep"); err == nil {
		t.Fatal("retention hold accepted without storage")
	}
	store := &RetentionStore{pool: coverageDatabase{result: coverageRowsAffected(2)}}
	for _, test := range []struct {
		class, id, reason string
		held              bool
		wantErr           bool
	}{
		{class: "invalid", id: "id", held: true, reason: "keep", wantErr: true},
		{class: "run", id: "", held: true, reason: "keep", wantErr: true},
		{class: "run", id: "id", held: true, wantErr: true},
		{class: "run", id: "id", held: true, reason: "keep"},
		{class: "run", id: "id"},
	} {
		err := store.SetLegalHold(ctx, test.class, test.id, test.held, test.reason)
		if (err != nil) != test.wantErr {
			t.Fatalf("SetLegalHold(%+v) error = %v", test, err)
		}
	}

	tx := coverageTx{result: coverageRowsAffected(2)}
	purge := &RetentionStore{pool: coverageDatabase{tx: tx}}
	result, err := purge.Purge(ctx, time.Time{}, DefaultRetentionPolicy(), 1)
	if err != nil || result != (RetentionResult{Runs: 2, DeadLetters: 2, AuditEvents: 2, RunnerMetrics: 2}) {
		t.Fatalf("Purge = %#v, %v", result, err)
	}
	critical := &RetentionStore{pool: coverageDatabase{tx: coverageTx{result: coverageRowsAffected(0)}}}
	if result, err := critical.PurgeCriticalRuns(ctx, time.Time{}, func() (float64, error) { return 10, nil }, 1); err != nil || result.Runs != 0 {
		t.Fatalf("PurgeCriticalRuns = %#v, %v", result, err)
	}
	if _, err := (&RetentionStore{}).Purge(ctx, time.Now(), DefaultRetentionPolicy(), 1); err == nil {
		t.Fatal("Purge accepted missing storage")
	}
	if _, err := purge.Purge(ctx, time.Now(), RetentionPolicy{}, 1); err == nil {
		t.Fatal("Purge accepted invalid policy")
	}
}

func TestDeadLetterValidationAndCryptoEdges(t *testing.T) {
	ctx := context.Background()
	var store *DeadLetterStore
	if err := store.Persist(ctx, DeadLetterRecord{}); err == nil {
		t.Fatal("nil dead-letter store accepted a record")
	}
	if _, _, err := store.List(ctx, DeadLetterFilter{}); err == nil {
		t.Fatal("nil dead-letter store listed records")
	}
	if _, _, err := store.Find(ctx, "id"); err == nil {
		t.Fatal("nil dead-letter store found a record")
	}
	if _, _, err := store.BeginRetry(ctx, "id"); err == nil {
		t.Fatal("nil dead-letter store began a retry")
	}
	if err := store.MarkRetryPublished(ctx, "id"); err == nil {
		t.Fatal("nil dead-letter store marked a retry")
	}
	if err := store.MarkRetryFailed(ctx, "id", "error", time.Now()); err == nil {
		t.Fatal("nil dead-letter store failed a retry")
	}
	if _, err := store.Reconcile(ctx, "id", "RECONCILED"); err == nil {
		t.Fatal("nil dead-letter store reconciled a record")
	}
	if _, err := store.Stats(ctx); err == nil {
		t.Fatal("nil dead-letter store returned stats")
	}
	if _, err := encryptDeadLetterPayload([]byte("short"), []byte("payload")); err == nil {
		t.Fatal("invalid encryption key was accepted")
	}
	if _, err := decryptDeadLetterPayload([]byte("01234567890123456789012345678901"), []byte("short")); err == nil {
		t.Fatal("short ciphertext was accepted")
	}
	if _, err := decryptDeadLetterPayload([]byte("bad"), []byte("short")); err == nil {
		t.Fatal("invalid decryption key was accepted")
	}
}

func TestSQLiteJSONContainsBranches(t *testing.T) {
	db := coverageSQLite(t)
	ctx := context.Background()
	for _, test := range []struct {
		container, subset string
		want              int
	}{
		{container: `{"a":1}`, subset: `{"a":1}`, want: 1},
		{container: `{"a":1}`, subset: `{"a":2}`, want: 0},
		{container: `{"a":1}`, subset: `{"b":1}`, want: 0},
		{container: `invalid`, subset: `{}`, want: 0},
	} {
		var got int
		if err := db.QueryRowContext(ctx, "SELECT json_contains(?, ?)", test.container, test.subset).Scan(&got); err != nil || got != test.want {
			t.Fatalf("json_contains(%q, %q) = %d, %v; want %d", test.container, test.subset, got, err, test.want)
		}
	}
}

func TestRunEventTransitionBranches(t *testing.T) {
	ctx := context.Background()
	reportedAt := time.Now().UTC()
	base := RunEventInput{RunID: "run", ReportedAt: reportedAt, Error: "timeout"}
	for _, eventType := range []string{"accepted", "started", "heartbeat", "completed", "cancelled"} {
		t.Run(eventType, func(t *testing.T) {
			event := base
			event.EventType = eventType
			event.ExitCode = intPtr(5)
			event.Metrics = map[string]int64{"max_memory_bytes": 10, "average_memory_bytes": 5}
			if err := (&RunStore{}).applyRunEventTransition(ctx, coverageTx{result: coverageRowsAffected(1)}, event, "attempt", "runner", "RUNNING", []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, eventType := range []string{"failed", "timed_out"} {
		t.Run(eventType, func(t *testing.T) {
			event := base
			event.EventType = eventType
			event.ExitCode = intPtr(5)
			tx := coverageTx{
				result: coverageRowsAffected(1),
				row: coverageScanner(func(dest ...any) error {
					*(dest[0].(*int)) = 1
					*(dest[1].(*int)) = 1
					*(dest[2].(*int)) = 10
					*(dest[3].(*float64)) = 2
					*(dest[4].(*[]byte)) = []byte(`[]`)
					*(dest[5].(*[]byte)) = []byte(`[]`)
					return nil
				}),
			}
			if err := (&RunStore{}).applyRunEventTransition(ctx, tx, event, "attempt", "runner", "RUNNING", []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		})
	}

	unknown := base
	unknown.EventType = "unknown"
	if err := (&RunStore{}).applyRunEventTransition(ctx, coverageTx{
		result: coverageRowsAffected(1),
		row: coverageScanner(func(dest ...any) error {
			*(dest[0].(*string)) = string(platform.RetryAmbiguous)
			return nil
		}),
	}, unknown, "attempt", "runner", "RUNNING", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := (&RunStore{}).applyRunEventTransition(ctx, coverageTx{result: coverageRowsAffected(1)}, RunEventInput{EventType: "unsupported"}, "attempt", "runner", "RUNNING", nil); err == nil {
		t.Fatal("unsupported run event was accepted")
	}

	for _, event := range []RunEventInput{
		{EventType: "log_chunk", EventChannel: "stderr", Result: "log", Sequence: 1, ReportedAt: reportedAt},
		{EventType: "log_chunk", EventChannel: "invalid", Result: "log", Sequence: 1},
		{EventType: "log_chunk", EventChannel: "stdout", Sequence: 1},
	} {
		err := (&RunStore{}).applyLogChunkEvent(ctx, coverageTx{result: coverageRowsAffected(1)}, event, "attempt")
		if event.EventChannel == "stderr" && err != nil {
			t.Fatal(err)
		}
		if event.EventChannel != "stderr" && err == nil {
			t.Fatalf("invalid log event %+v was accepted", event)
		}
	}
}

func TestRunUnknownStateBranches(t *testing.T) {
	ctx := context.Background()
	event := RunEventInput{RunID: "run", ReportedAt: time.Now().UTC()}
	for _, policy := range []platform.AmbiguityPolicy{platform.RetryAmbiguous, platform.FailedAmbiguous, platform.ManualAmbiguous} {
		if err := applyUnknownRunState(ctx, coverageTx{result: coverageRowsAffected(1)}, event, policy); err != nil {
			t.Fatalf("policy %q: %v", policy, err)
		}
	}
	if err := applyUnknownRunState(ctx, coverageTx{result: coverageRowsAffected(1)}, event, "invalid"); err == nil {
		t.Fatal("invalid ambiguity policy was accepted")
	}
}

func TestRunGuardBranches(t *testing.T) {
	ctx := context.Background()
	runs := &RunStore{}
	if _, _, err := runs.ClaimWaiting(ctx, nil); err == nil {
		t.Fatal("nil dispatch builder was accepted")
	}
	if _, _, err := runs.ClaimCancelling(ctx, nil); err == nil {
		t.Fatal("nil cancellation builder was accepted")
	}
	if _, _, err := runs.ClaimStart(ctx, StartClaimInput{}); err == nil {
		t.Fatal("incomplete start claim was accepted")
	}
	if err := runs.AuthorizeSecretRequest(ctx, SecretRequestInput{}); err == nil {
		t.Fatal("incomplete secret request was accepted")
	}
	if err := runs.ReconcileTimedOutDispatches(ctx, time.Time{}); err == nil {
		t.Fatal("zero dispatch reconciliation time was accepted")
	}
	if err := runs.ReconcileStaleCancellations(ctx, time.Time{}); err == nil {
		t.Fatal("zero cancellation reconciliation cutoff was accepted")
	}
	if _, _, err := (&RunStore{pool: coverageDatabase{result: coverageRowsAffected(0)}}).RequestCancellation(ctx, "run", " "); err == nil {
		t.Fatal("blank cancellation reason was accepted")
	}
	db := coverageDatabase{
		row: coverageScanner(func(...any) error { return pgx.ErrNoRows }),
		tx:  coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows }), rows: &coverageRows{}},
	}
	runs.pool = db
	if _, changed, err := runs.RequestCancellation(ctx, "run", "stop"); err != nil || changed {
		t.Fatalf("missing cancellation = changed %v err %v", changed, err)
	}
	if _, _, err := runs.ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return []byte("order"), nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runs.ClaimCancelling(ctx, func(CancellationCandidate) ([]byte, error) { return []byte("order"), nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runs.ClaimStart(ctx, StartClaimInput{RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Attempt: 1, FencingToken: 1, ExecutionSpecDigest: "digest"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.AuthorizeSecretRequest(ctx, SecretRequestInput{OrderID: "order", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", ExecutionSpecDigest: "digest", Attempt: 1, FencingToken: 1, SecretRefs: map[string]string{"key": "value"}}); err == nil {
		t.Fatal("missing secret request was authorized")
	}
	if err := runs.ReconcileTimedOutDispatches(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := runs.ReconcileStaleCancellations(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := runs.Retry(ctx, "run", "retry"); err != nil || changed {
		t.Fatalf("missing retry = changed %v err %v", changed, err)
	}
}

func TestRunnerAndTaskGuardBranches(t *testing.T) {
	ctx := context.Background()
	runners := &RunnerStore{}
	if _, err := runners.ConsumeEnrollmentWithKey(ctx, "token", time.Now(), "", make([]byte, 32)); err == nil {
		t.Fatal("incomplete enrollment key was accepted")
	}
	if _, err := runners.ConsumeEnrollmentWithKey(ctx, "token", time.Now(), "key", make([]byte, 1)); err == nil {
		t.Fatal("short enrollment public key was accepted")
	}
	if err := runners.HeartbeatWithKeyAndCapacity(ctx, "", "boot", time.Now(), 1, "key", make([]byte, 32)); err == nil {
		t.Fatal("incomplete heartbeat was accepted")
	}
	if err := runners.HeartbeatWithKeyAndCapacityAndMetrics(ctx, "runner", "boot", time.Now(), 1, RunnerMetricsSample{CPUPercent: -1}, "key", make([]byte, 32)); err == nil {
		t.Fatal("invalid heartbeat metrics were accepted")
	}
	if err := (&RunnerStore{pool: coverageDatabase{result: coverageRowsAffected(0)}}).EnsurePool(ctx, "pool", "Pool"); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := (&RunStore{pool: coverageDatabase{result: coverageRowsAffected(0)}}).RequestCancellation(ctx, "run", "stop"); err != nil || changed {
		t.Fatalf("no-op cancellation = changed %v err %v", changed, err)
	}
	if got := maxRunnerCapacity(0); got != DefaultRunnerCapacity {
		t.Fatalf("default runner capacity = %d", got)
	}
}

func TestSQLiteRunOutcomeBranches(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	runners := NewRunnerRepository(db)
	if err := runners.EnsurePool(ctx, "outcome-pool", "Outcome"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runners (id, pool_id, name, capacity, capabilities) VALUES (?, ?, ?, 10, '{}')`, "outcome-runner", "outcome-pool", "Outcome runner"); err != nil {
		t.Fatal(err)
	}
	if err := runners.CreateSession(ctx, "outcome-runner", "boot"); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(db)
	if _, err := tasks.Create(ctx, TaskDefinition{ID: "outcome-task", Name: "Outcome task", RunnerPoolID: "outcome-pool", Command: []string{"true"}, Enabled: true, MaxAttempts: 2, RetryableExitCodes: []int{5}, AmbiguityPolicy: string(platform.RetryAmbiguous)}); err != nil {
		t.Fatal(err)
	}
	runs := NewRunRepository(db)
	apply := func(runID, eventType, eventID string, exitCode *int, eventError string) {
		run, err := runs.Create(ctx, RunDefinition{ID: runID, TaskID: "outcome-task", ScheduledFor: time.Now().UTC().Add(-time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		candidate, acquired, err := runs.ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return []byte("order"), nil })
		if err != nil || !acquired || candidate.RunID != run.ID {
			t.Fatalf("claim %s = %#v acquired=%v err=%v", runID, candidate, acquired, err)
		}
		base := RunEventInput{RunID: runID, RunnerID: candidate.RunnerID, RunnerSessionID: candidate.RunnerSessionID, LeaseToken: candidate.LeaseToken, FencingToken: candidate.FencingToken, Attempt: int64(candidate.AttemptNumber), Envelope: []byte("signed")}
		for _, event := range []RunEventInput{
			{EventID: eventID + "-accepted", EventType: "accepted", Sequence: 1},
			{EventID: eventID + "-started", EventType: "started", Sequence: 2},
		} {
			event.RunID, event.RunnerID, event.RunnerSessionID, event.LeaseToken, event.FencingToken, event.Attempt, event.Envelope = base.RunID, base.RunnerID, base.RunnerSessionID, base.LeaseToken, base.FencingToken, base.Attempt, base.Envelope
			if err := runs.ApplyRunEvent(ctx, event); err != nil {
				t.Fatal(err)
			}
		}
		event := base
		event.EventID, event.EventType, event.Sequence, event.ExitCode, event.Error = eventID, eventType, 3, exitCode, eventError
		if err := runs.ApplyRunEvent(ctx, event); err != nil {
			t.Fatalf("apply %s: %v", eventType, err)
		}
		if err := runs.ApplyRunEvent(ctx, event); err != nil {
			t.Fatalf("duplicate %s: %v", eventType, err)
		}
		stale := event
		stale.EventID, stale.Sequence = eventID+"-stale", 1
		if err := runs.ApplyRunEvent(ctx, stale); err != nil {
			t.Fatalf("stale %s: %v", eventType, err)
		}
		if runID == "outcome-retry" {
			if _, err := db.ExecContext(ctx, `UPDATE runs SET state = 'FAILED' WHERE id = ?`, runID); err != nil {
				t.Fatal(err)
			}
		}
	}
	code := 5
	apply("outcome-retry", "failed", "outcome-retry-event", &code, "retryable")
	code = 1
	apply("outcome-failed", "failed", "outcome-failed-event", &code, "not retryable")
	apply("outcome-timeout", "timed_out", "outcome-timeout-event", nil, "timeout")
	apply("outcome-cancelled", "cancelled", "outcome-cancelled-event", nil, "cancelled")
	apply("outcome-unknown", "unknown", "outcome-unknown-event", nil, "runner restart")
}

func TestSQLiteUserAndRunnerEdges(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	users := NewUserRepository(db)
	if err := users.Create(ctx, UserRecord{ID: "user-no-password", Username: "no-password", Email: "no-password@example.com", Status: StatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := users.List(ctx, "invalid"); err == nil {
		t.Fatal("invalid user list status was accepted")
	}
	if _, _, err := users.ListPage(ctx, "invalid", "", nil, 1, 0); err == nil {
		t.Fatal("invalid user page status was accepted")
	}
	if hash, found, err := users.PasswordHash(ctx, "missing"); err != nil || found || hash != "" {
		t.Fatalf("missing password = %q found=%v err=%v", hash, found, err)
	}
	if err := users.UpdateDisplayName(ctx, "missing", ""); err == nil {
		t.Fatal("missing user display name update succeeded")
	}
	if err := users.SetStatus(ctx, "user-no-password", "invalid"); err == nil {
		t.Fatal("invalid user status update succeeded")
	}
	if err := users.ProvisionLocal(ctx, UserRecord{ID: "user-local-no-admin", Username: "local-no-admin", Email: "local-no-admin@example.com", Enabled: false}, "", "role-default", ""); err != nil {
		t.Fatal(err)
	}

	runners := NewRunnerRepository(db)
	if err := runners.EnsurePool(ctx, "pool-runner-edges", "Runner edges"); err != nil {
		t.Fatal(err)
	}
	if err := runners.CreateEnrollment(ctx, RunnerRecord{ID: "runner-by-name", Name: "By name", Pool: "Runner edges"}, RunnerEnrollmentRecord{ID: "enrollment-by-name", TokenHash: "00ff", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ConsumeEnrollment(ctx, "00ff", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ConsumeEnrollment(ctx, "00ff", time.Now().UTC()); err == nil {
		t.Fatal("used enrollment was accepted")
	}
	if err := runners.CreateEnrollment(ctx, RunnerRecord{ID: "runner-delete", Name: "Delete", PoolID: "pool-runner-edges"}, RunnerEnrollmentRecord{ID: "enrollment-delete", TokenHash: "abcd", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if deleted, err := runners.Delete(ctx, "runner-delete"); err != nil || !deleted {
		t.Fatalf("runner delete = %v, %v", deleted, err)
	}
	if archived, err := runners.Archive(ctx, "runner-by-name"); err != nil || !archived {
		t.Fatalf("runner archive = %v, %v", archived, err)
	}
	if archived, err := runners.Archive(ctx, "runner-by-name"); err != nil || !archived {
		t.Fatalf("idempotent runner archive = %v, %v", archived, err)
	}
}

func TestSQLiteScheduleAndDeadLetterEdges(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	if valueOrID(nil, "id") != "id" || valueOrID(stringPtr("name"), "id") != "name" || valueOrEmpty(nil) != "" || valueOrEmpty(stringPtr("kind")) != "kind" {
		t.Fatal("schedule nullable value helpers returned the wrong value")
	}
	runners := NewRunnerRepository(db)
	if err := runners.EnsurePool(ctx, "pool-schedule-edge", "Schedule edge"); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(db)
	if _, err := tasks.Create(ctx, TaskDefinition{ID: "task-schedule-edge", Name: "Schedule edge", RunnerPoolID: "pool-schedule-edge", Command: []string{"true"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	schedules := NewScheduleRepository(db)
	if _, err := schedules.Create(ctx, ScheduleDefinition{ID: "schedule-initialized", Name: "Initialized", TaskID: "task-schedule-edge", Expression: "* * * * *", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if runID, changed, err := schedules.CreateDueRun(ctx, time.Now().UTC(), func(due DueScheduleRecord) (time.Time, error) { return due.NextFireAt.Add(time.Minute), nil }); err != nil || !changed || runID != "" {
		t.Fatalf("initialized schedule = %q changed=%v err=%v", runID, changed, err)
	}
	deadLetters := NewDeadLetterRepository(db, []byte("application-secret"))
	if err := deadLetters.Persist(ctx, DeadLetterRecord{Stream: "edge-stream", Consumer: "edge-consumer", Subject: "edge-subject", MessageID: "edge-message", Payload: []byte("payload"), Attempts: 1, Error: string([]byte{'i', 'n', 'v', 0xff})}); err != nil {
		t.Fatal(err)
	}
	if err := deadLetters.MarkRetryFailed(ctx, "missing", string(make([]byte, 5000)), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := deadLetters.BeginRetry(ctx, "missing"); err != nil || claimed {
		t.Fatalf("missing dead-letter retry = claimed=%v err=%v", claimed, err)
	}
	if _, err := deadLetters.Reconcile(ctx, "missing", "INVALID"); err == nil {
		t.Fatal("invalid dead-letter state was accepted")
	}
}

func TestEncryptedSecretDeleteBranches(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name string
		row  *coverageScanSequence
		want error
	}{
		{name: "deleted", row: &coverageScanSequence{scans: []coverageScanner{func(dest ...any) error { *(dest[0].(*string)) = "secret"; return nil }}}},
		{name: "missing", row: &coverageScanSequence{scans: []coverageScanner{func(...any) error { return pgx.ErrNoRows }, func(dest ...any) error { *(dest[0].(*bool)) = false; return nil }}}, want: ErrEncryptedSecretNotFound},
		{name: "in use", row: &coverageScanSequence{scans: []coverageScanner{func(...any) error { return pgx.ErrNoRows }, func(dest ...any) error { *(dest[0].(*bool)) = true; return nil }}}, want: ErrEncryptedSecretInUse},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &EncryptedSecretStore{pool: coverageDatabase{row: test.row}}
			err := store.Delete(ctx, "secret")
			if !errors.Is(err, test.want) {
				t.Fatalf("Delete error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestScheduleDueRunBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	future := now.Add(time.Minute)
	makeStore := func(policy string, active, max int, nextFire *time.Time) *ScheduleStore {
		return &ScheduleStore{pool: coverageDatabase{tx: coverageTx{
			result: coverageRowsAffected(1),
			row: coverageScanner(func(dest ...any) error {
				*(dest[0].(*string)) = "schedule"
				*(dest[1].(*string)) = "task"
				*(dest[2].(*string)) = "task-v1"
				*(dest[3].(*string)) = "schedule-v1"
				*(dest[4].(*string)) = "* * * * *"
				*(dest[5].(*string)) = "UTC"
				*(dest[6].(*string)) = "SKIP_ALL"
				*(dest[7].(*int)) = 10
				*(dest[8].(*int)) = 60
				*(dest[9].(*string)) = policy
				*(dest[10].(*int)) = max
				*(dest[11].(*int)) = active
				*(dest[12].(**time.Time)) = nextFire
				return nil
			}),
			rows: &coverageRows{},
		}}}
	}
	if _, changed, err := (&ScheduleStore{pool: coverageDatabase{tx: coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows }), rows: &coverageRows{}}}}).createDueRun(ctx, now, func(DueScheduleRecord) (time.Time, error) { return future, nil }); err != nil || changed {
		t.Fatalf("missing due schedule = changed %v err %v", changed, err)
	}
	if _, changed, err := makeStore("ALLOW", 1, 1, &future).createDueRun(ctx, now, func(DueScheduleRecord) (time.Time, error) { return future.Add(time.Minute), nil }); err != nil || changed {
		t.Fatalf("concurrency-limited schedule = changed %v err %v", changed, err)
	}
	if _, changed, err := makeStore("QUEUE", 1, 10, &future).createDueRun(ctx, now, func(DueScheduleRecord) (time.Time, error) { return future.Add(time.Minute), nil }); err != nil || changed {
		t.Fatalf("queued schedule = changed %v err %v", changed, err)
	}
	if _, changed, err := makeStore("SKIP", 1, 10, &future).createDueRun(ctx, now, func(DueScheduleRecord) (time.Time, error) { return future.Add(time.Minute), nil }); err != nil || !changed {
		t.Fatalf("skipped schedule = changed %v err %v", changed, err)
	}
	if _, changed, err := makeStore("REPLACE", 1, 10, &future).createDueRun(ctx, now, func(DueScheduleRecord) (time.Time, error) { return future.Add(time.Minute), nil }); err != nil || !changed {
		t.Fatalf("replacement schedule = changed %v err %v", changed, err)
	}
	if _, _, err := makeStore("ALLOW", 0, 0, &future).createDueRun(ctx, now, func(DueScheduleRecord) (time.Time, error) { return time.Time{}, errors.New("clock failed") }); err == nil {
		t.Fatal("next-fire error was ignored")
	}
	if _, _, err := makeStore("ALLOW", 0, 0, &future).createDueRun(ctx, now, func(due DueScheduleRecord) (time.Time, error) { return due.NextFireAt, nil }); err == nil {
		t.Fatal("non-advancing next-fire was accepted")
	}
	if _, changed, err := makeStore("ALLOW", 0, 0, nil).createDueRun(ctx, now, func(due DueScheduleRecord) (time.Time, error) { return due.NextFireAt.Add(time.Minute), nil }); err != nil || !changed {
		t.Fatalf("uninitialized schedule = changed %v err %v", changed, err)
	}
}

func TestSessionRotateBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	scanSession := func(hash string, expires time.Time, revoked *time.Time) coverageScanner {
		return func(dest ...any) error {
			*(dest[0].(*string)) = "session"
			*(dest[1].(*string)) = "user"
			*(dest[2].(*string)) = hash
			*(dest[3].(*time.Time)) = now
			*(dest[4].(*time.Time)) = expires
			*(dest[5].(*string)) = "family"
			*(dest[6].(*string)) = "agent"
			*(dest[7].(*string)) = "ip"
			*(dest[8].(*time.Time)) = now
			*(dest[9].(**time.Time)) = revoked
			return nil
		}
	}
	replacement := SessionRecord{ID: "replacement", RefreshTokenHash: "new", AccessExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(time.Hour), LastSeenAt: now}
	if err := (&SessionStore{pool: coverageDatabase{tx: coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows })}}}).Rotate(ctx, "missing", "old", replacement); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("missing rotation error = %v", err)
	}
	expired := &SessionStore{pool: coverageDatabase{tx: coverageTx{result: coverageRowsAffected(1), row: scanSession("old", now.Add(-time.Minute), nil)}}}
	if err := expired.Rotate(ctx, "session", "old", replacement); !errors.Is(err, ErrSessionReplay) {
		t.Fatalf("expired rotation error = %v", err)
	}
	revokedAt := now.Add(-time.Minute)
	replayed := &SessionStore{pool: coverageDatabase{tx: coverageTx{result: coverageRowsAffected(1), row: scanSession("old", now.Add(time.Hour), &revokedAt)}}}
	if err := replayed.Rotate(ctx, "session", "old", replacement); !errors.Is(err, ErrSessionReplay) {
		t.Fatalf("revoked rotation error = %v", err)
	}
	wrongHash := &SessionStore{pool: coverageDatabase{tx: coverageTx{result: coverageRowsAffected(1), row: scanSession("old", now.Add(time.Hour), nil)}}}
	if err := wrongHash.Rotate(ctx, "session", "wrong", replacement); !errors.Is(err, ErrSessionReplay) {
		t.Fatalf("wrong hash rotation error = %v", err)
	}
	valid := &SessionStore{pool: coverageDatabase{tx: coverageTx{result: coverageRowsAffected(1), row: scanSession("old", now.Add(time.Hour), nil)}}}
	if err := valid.Rotate(ctx, "session", "old", replacement); err != nil {
		t.Fatalf("valid rotation error = %v", err)
	}
}

func TestDeadLetterRetryAndStatsBranches(t *testing.T) {
	ctx := context.Background()
	scanRetry := func(state, delivery string, attempts int, available, published *time.Time) coverageScanner {
		return func(dest ...any) error {
			*(dest[0].(*string)) = "subject"
			*(dest[1].(*string)) = "message"
			*(dest[2].(*[]byte)) = []byte("ciphertext")
			*(dest[3].(*string)) = state
			*(dest[4].(*string)) = delivery
			*(dest[5].(*int)) = attempts
			*(dest[6].(**time.Time)) = available
			*(dest[7].(**time.Time)) = published
			return nil
		}
	}
	now := time.Now().UTC()
	for _, test := range []struct {
		name, state, delivery string
		attempts              int
		available, published  *time.Time
	}{
		{name: "terminal", state: "RECONCILED"},
		{name: "missing delivery", state: "RETRY_QUEUED", attempts: 1},
		{name: "published", state: "RETRY_QUEUED", delivery: "delivery", attempts: 1, published: &now},
		{name: "at limit", state: "RETRY_QUEUED", delivery: "delivery", attempts: MaxDeadLetterRetryAttempts},
		{name: "not available", state: "RETRY_QUEUED", delivery: "delivery", attempts: 1, available: func() *time.Time { value := now.Add(time.Minute); return &value }()},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &DeadLetterStore{pool: coverageDatabase{tx: coverageTx{row: scanRetry(test.state, test.delivery, test.attempts, test.available, test.published)}}}
			if _, claimed, err := store.BeginRetry(ctx, "dead-letter"); err != nil || claimed {
				t.Fatalf("BeginRetry claimed=%v err=%v", claimed, err)
			}
		})
	}
	valid := &DeadLetterStore{pool: coverageDatabase{tx: coverageTx{result: coverageRowsAffected(1), row: scanRetry("RETRY_QUEUED", "delivery", 1, nil, nil)}}}
	if _, claimed, err := valid.BeginRetry(ctx, "dead-letter"); err != nil || !claimed {
		t.Fatalf("re-queued retry claimed=%v err=%v", claimed, err)
	}
	stats := &DeadLetterStore{pool: coverageDatabase{row: coverageScanner(func(dest ...any) error { *(dest[0].(*int64)) = 1; *(dest[1].(*float64)) = -1; return nil })}}
	if got, err := stats.Stats(ctx); err != nil || got.OldestAgeSeconds != 0 {
		t.Fatalf("negative dead-letter age = %#v err=%v", got, err)
	}
}

func TestRepositoryDatabaseErrorBranches(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("database failure")
	rowFailure := coverageScanner(func(...any) error { return failure })
	db := coverageDatabase{err: failure, beginErr: failure, row: rowFailure}
	if err := NewAuditRepository(db).Append(ctx, AuditEventRecord{ID: "id"}); err == nil {
		t.Fatal("audit append ignored database error")
	}
	if _, _, err := NewAuditRepository(db).Query(ctx, AuditFilter{}); err == nil {
		t.Fatal("audit query ignored database error")
	}
	config := NewConfigStore(db)
	if err := config.SetIfAbsent(ctx, "WEB_ORIGIN", "value"); err == nil {
		t.Fatal("config set-if-absent ignored database error")
	}
	if err := config.SetAuthenticationSettings(ctx, true, true, "role"); err == nil {
		t.Fatal("authentication settings ignored database error")
	}
	dead := NewDeadLetterRepository(db, []byte("secret"))
	if _, _, err := dead.List(ctx, DeadLetterFilter{}); err == nil {
		t.Fatal("dead-letter list ignored database error")
	}
	if _, _, err := dead.Find(ctx, "id"); err == nil {
		t.Fatal("dead-letter find ignored database error")
	}
	if _, _, err := dead.BeginRetry(ctx, "id"); err == nil {
		t.Fatal("dead-letter retry ignored database error")
	}
	if err := dead.MarkRetryPublished(ctx, "id"); err == nil {
		t.Fatal("dead-letter publish ignored database error")
	}
	if err := dead.MarkRetryFailed(ctx, "id", "failure", time.Now()); err == nil {
		t.Fatal("dead-letter failure ignored database error")
	}
	if _, err := dead.Reconcile(ctx, "id", "RECONCILED"); err == nil {
		t.Fatal("dead-letter reconcile ignored database error")
	}
	if _, err := dead.Stats(ctx); err == nil {
		t.Fatal("dead-letter stats ignored database error")
	}
	secrets := NewEncryptedSecretRepository(db)
	if err := secrets.Upsert(ctx, EncryptedSecretRecord{ID: "id"}); err == nil {
		t.Fatal("secret upsert ignored database error")
	}
	if _, _, err := secrets.Find(ctx, "id"); err == nil {
		t.Fatal("secret find ignored database error")
	}
	if err := secrets.SetIntegrityStatus(ctx, "id", SecretIntegrityValid, time.Now()); err == nil {
		t.Fatal("secret status ignored database error")
	}
	if _, err := secrets.ListStatuses(ctx); err == nil {
		t.Fatal("secret status list ignored database error")
	}
	if err := secrets.Delete(ctx, "id"); err == nil {
		t.Fatal("secret delete ignored database error")
	}
	exitCodes := NewExitCodeRepository(db)
	if _, err := exitCodes.List(ctx); err == nil {
		t.Fatal("exit-code list ignored database error")
	}
	if _, err := exitCodes.Create(ctx, 42, "meaning"); err == nil {
		t.Fatal("exit-code create ignored database error")
	}
	if _, err := exitCodes.Update(ctx, 42, 43, "meaning"); err == nil {
		t.Fatal("exit-code update ignored database error")
	}
	if err := exitCodes.Delete(ctx, 42); err == nil {
		t.Fatal("exit-code delete ignored database error")
	}
	variables := NewGlobalVariableRepository(db)
	if _, err := variables.List(ctx); err == nil {
		t.Fatal("global-variable list ignored database error")
	}
	if _, err := variables.Create(ctx, "id", "name", "value"); err == nil {
		t.Fatal("global-variable create ignored database error")
	}
	if _, err := variables.Update(ctx, "id", "name", "value"); err == nil {
		t.Fatal("global-variable update ignored database error")
	}
	if err := variables.Delete(ctx, "id"); err == nil {
		t.Fatal("global-variable delete ignored database error")
	}
	resources := NewResourceRepository(db)
	if _, err := resources.List(ctx); err == nil {
		t.Fatal("resource list ignored database error")
	}
	if _, _, err := resources.Find(ctx, "id"); err == nil {
		t.Fatal("resource find ignored database error")
	}
	if err := resources.Create(ctx, "id", "name", "kind"); err == nil {
		t.Fatal("resource create ignored database error")
	}
	if err := resources.Delete(ctx, "id"); err == nil {
		t.Fatal("resource delete ignored database error")
	}
	if err := resources.Release(ctx, "id", "holder", 1); err == nil {
		t.Fatal("resource release ignored database error")
	}
	retention := NewRetentionRepository(db)
	if err := retention.SetLegalHold(ctx, "run", "id", true, "keep"); err == nil {
		t.Fatal("retention hold ignored database error")
	}
	if _, err := retention.Purge(ctx, time.Now(), DefaultRetentionPolicy(), 1); err == nil {
		t.Fatal("retention purge ignored database error")
	}
	if _, err := retention.PurgeCriticalRuns(ctx, time.Now(), func() (float64, error) { return 0, failure }, 1); err == nil {
		t.Fatal("critical retention ignored database error")
	}
	roles := NewRoleRepository(db)
	if err := roles.Ensure(ctx, "id", "name", "", false, nil); err == nil {
		t.Fatal("role ensure ignored database error")
	}
	if _, err := roles.List(ctx); err == nil {
		t.Fatal("role list ignored database error")
	}
	if _, _, err := roles.FindByID(ctx, "id"); err == nil {
		t.Fatal("role find ignored database error")
	}
	if err := roles.Rename(ctx, "id", "name"); err == nil {
		t.Fatal("role rename ignored database error")
	}
	if err := roles.ReplacePermissions(ctx, "id", nil); err == nil {
		t.Fatal("role permission replacement ignored database error")
	}
	if err := roles.Delete(ctx, "id"); err == nil {
		t.Fatal("role delete ignored database error")
	}
	if err := roles.UnassignSource(ctx, "user", "role", "manual"); err == nil {
		t.Fatal("role unassign ignored database error")
	}
	if err := roles.ReplaceSourceAssignments(ctx, "role", "manual", nil); err == nil {
		t.Fatal("role source replacement ignored database error")
	}
	if err := roles.ReplaceSSOAssignments(ctx, "user", "provider", nil); err == nil {
		t.Fatal("role SSO replacement ignored database error")
	}
	if _, _, err := roles.UserRoles(ctx, "user"); err == nil {
		t.Fatal("user roles ignored database error")
	}
	if _, err := roles.EffectivePermissions(ctx, "user"); err == nil {
		t.Fatal("effective permissions ignored database error")
	}
}

func TestRunClaimStartBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	input := StartClaimInput{RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Attempt: 1, FencingToken: 7, ExecutionSpecDigest: "digest"}
	scan := func(runState, attemptState, runnerID, sessionID, lease, digest string, fencing int64, leaseUntil, deadline time.Time) coverageScanner {
		return func(dest ...any) error {
			*(dest[0].(*string)) = runState
			*(dest[1].(*string)) = attemptState
			*(dest[2].(*string)) = runnerID
			*(dest[3].(*string)) = sessionID
			*(dest[4].(*string)) = lease
			*(dest[5].(*int64)) = fencing
			*(dest[6].(*string)) = digest
			*(dest[7].(*time.Time)) = leaseUntil
			*(dest[8].(*time.Time)) = deadline
			*(dest[9].(*time.Time)) = now
			return nil
		}
	}
	future := now.Add(time.Minute)
	valid := func(tx coverageTx) *RunStore {
		if tx.row == nil {
			tx.row = scan("DISPATCHED", "ACCEPTED", "runner", "session", "lease", "digest", 7, future, future)
		}
		return &RunStore{pool: coverageDatabase{tx: tx}}
	}
	testRunClaimStartResults(t, ctx, input, valid, scan, future)
}

func testRunClaimStartResults(t *testing.T, ctx context.Context, input StartClaimInput, valid func(coverageTx) *RunStore, scan func(string, string, string, string, string, string, int64, time.Time, time.Time) coverageScanner, future time.Time) {
	if _, claimed, err := (&RunStore{pool: coverageDatabase{beginErr: errors.New("begin")}}).ClaimStart(ctx, input); err == nil || claimed {
		t.Fatal("begin failure was ignored")
	}
	if _, claimed, err := valid(coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows })}).ClaimStart(ctx, input); err != nil || claimed {
		t.Fatalf("missing claim = claimed %v err %v", claimed, err)
	}
	if _, claimed, err := valid(coverageTx{row: scan("DISPATCHED", "ACCEPTED", "other", "session", "lease", "digest", 7, future, future)}).ClaimStart(ctx, input); err != nil || claimed {
		t.Fatalf("mismatched claim = claimed %v err %v", claimed, err)
	}
	if claimedAt, claimed, err := valid(coverageTx{row: scan("RUNNING", "RUNNING", "runner", "session", "lease", "digest", 7, future, future)}).ClaimStart(ctx, input); err != nil || !claimed || claimedAt.IsZero() {
		t.Fatalf("already-running claim = %v claimed=%v err=%v", claimedAt, claimed, err)
	}
	if _, claimed, err := valid(coverageTx{row: scan("WAITING", "DISPATCHED", "runner", "session", "lease", "digest", 7, future, future)}).ClaimStart(ctx, input); err != nil || claimed {
		t.Fatalf("invalid state claim = claimed %v err %v", claimed, err)
	}
	if claimedAt, claimed, err := valid(coverageTx{result: coverageRowsAffected(1), row: scan("DISPATCHED", "ACCEPTED", "runner", "session", "lease", "digest", 7, future, future)}).ClaimStart(ctx, input); err != nil || !claimed || claimedAt.IsZero() {
		t.Fatalf("valid claim = %v claimed=%v err=%v", claimedAt, claimed, err)
	}
	if _, claimed, err := valid(coverageTx{err: errors.New("update")}).ClaimStart(ctx, input); err == nil || claimed {
		t.Fatal("claim update failure was ignored")
	}
	if _, claimed, err := valid(coverageTx{result: coverageRowsAffected(1), commitErr: errors.New("commit")}).ClaimStart(ctx, input); err == nil || claimed {
		t.Fatal("claim commit failure was ignored")
	}
}

func TestRunSecretAuthorizationBranches(t *testing.T) {
	ctx := context.Background()
	input := SecretRequestInput{OrderID: "order", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", ExecutionSpecDigest: "digest", Attempt: 1, FencingToken: 1, SecretRefs: map[string]string{"token": "secret"}}
	valid := func(raw []byte) *RunStore {
		return &RunStore{pool: coverageDatabase{row: coverageScanner(func(dest ...any) error { *(dest[0].(*[]byte)) = raw; return nil })}}
	}
	if err := valid([]byte(`{"token":"secret"}`)).AuthorizeSecretRequest(ctx, input); err != nil {
		t.Fatalf("matching secret request rejected: %v", err)
	}
	if err := valid([]byte(`{"token":"other"}`)).AuthorizeSecretRequest(ctx, input); err == nil {
		t.Fatal("mismatched secret request was authorized")
	}
	if err := valid([]byte("invalid")).AuthorizeSecretRequest(ctx, input); err == nil {
		t.Fatal("invalid stored secret references were authorized")
	}
	if err := (&RunStore{pool: coverageDatabase{row: coverageScanner(func(...any) error { return errors.New("query") })}}).AuthorizeSecretRequest(ctx, input); err == nil {
		t.Fatal("secret authorization query failure was ignored")
	}
}

func TestRunReconciliationCandidateBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	rows := &coverageRows{rows: []func(...any) error{
		func(dest ...any) error {
			*(dest[0].(*string)) = "attempt-unknown"
			*(dest[1].(*string)) = "run-unknown"
			*(dest[2].(*string)) = "runner"
			*(dest[3].(*string)) = "DISPATCHED"
			*(dest[4].(*int64)) = 0
			*(dest[5].(*bool)) = false
			return nil
		},
		func(dest ...any) error {
			*(dest[0].(*string)) = "attempt-failed"
			*(dest[1].(*string)) = "run-failed"
			*(dest[2].(*string)) = "runner"
			*(dest[3].(*string)) = "ACCEPTED"
			*(dest[4].(*int64)) = 4
			*(dest[5].(*bool)) = true
			return nil
		},
	}}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{rows: rows, result: coverageRowsAffected(1)}}}).ReconcileTimedOutDispatches(ctx, now); err != nil {
		t.Fatalf("timed-out reconciliation failed: %v", err)
	}
	staleRows := &coverageRows{rows: []func(...any) error{
		func(dest ...any) error {
			*(dest[0].(*string)) = "cancel-unknown"
			*(dest[1].(*string)) = "run-unknown"
			*(dest[2].(*string)) = "runner"
			*(dest[3].(*int64)) = 0
			*(dest[4].(*string)) = ""
			return nil
		},
		func(dest ...any) error {
			*(dest[0].(*string)) = "cancelled"
			*(dest[1].(*string)) = "run-cancelled"
			*(dest[2].(*string)) = "runner"
			*(dest[3].(*int64)) = 4
			*(dest[4].(*string)) = "runner archived"
			return nil
		},
	}}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{rows: staleRows, result: coverageRowsAffected(1)}}}).ReconcileStaleCancellations(ctx, now); err != nil {
		t.Fatalf("stale cancellation reconciliation failed: %v", err)
	}
}

func TestRunClaimCancellingBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	input := func(dest ...any) error {
		*(dest[0].(*string)) = "run"
		*(dest[1].(*string)) = "task"
		*(dest[2].(*string)) = "attempt"
		*(dest[3].(*string)) = "runner"
		*(dest[4].(*string)) = "session"
		*(dest[5].(*string)) = "lease"
		*(dest[6].(*int)) = 1
		*(dest[7].(*int64)) = 2
		*(dest[8].(*time.Time)) = now.Add(time.Minute)
		*(dest[9].(*string)) = "stop"
		return nil
	}
	build := func(CancellationCandidate) ([]byte, error) { return []byte("order"), nil }
	newStore := func(tx coverageTx) *RunStore { return &RunStore{pool: coverageDatabase{tx: tx}} }
	testRunClaimCancellingResults(t, ctx, input, build, newStore)
}

func testRunClaimCancellingResults(t *testing.T, ctx context.Context, input func(...any) error, build func(CancellationCandidate) ([]byte, error), newStore func(coverageTx) *RunStore) {
	if _, claimed, err := (&RunStore{pool: coverageDatabase{beginErr: errors.New("begin")}}).ClaimCancelling(ctx, build); err == nil || claimed {
		t.Fatal("begin failure was ignored")
	}
	if _, claimed, err := newStore(coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows })}).ClaimCancelling(ctx, build); err != nil || claimed {
		t.Fatalf("missing cancellation = claimed %v err %v", claimed, err)
	}
	if _, claimed, err := newStore(coverageTx{row: coverageScanner(func(...any) error { return errors.New("scan") })}).ClaimCancelling(ctx, build); err == nil || claimed {
		t.Fatal("cancellation scan failure was ignored")
	}
	if _, claimed, err := newStore(coverageTx{row: coverageScanner(input)}).ClaimCancelling(ctx, func(CancellationCandidate) ([]byte, error) { return nil, errors.New("build") }); err == nil || claimed {
		t.Fatal("cancellation build failure was ignored")
	}
	if _, claimed, err := newStore(coverageTx{row: coverageScanner(input)}).ClaimCancelling(ctx, func(CancellationCandidate) ([]byte, error) { return nil, nil }); err == nil || claimed {
		t.Fatal("empty cancellation order was accepted")
	}
	if _, claimed, err := newStore(coverageTx{row: coverageScanner(input), execErr: errors.New("exec")}).ClaimCancelling(ctx, build); err == nil || claimed {
		t.Fatal("cancellation update failure was ignored")
	}
	if _, claimed, err := newStore(coverageTx{row: coverageScanner(input), result: coverageRowsAffected(1), commitErr: errors.New("commit")}).ClaimCancelling(ctx, build); err == nil || claimed {
		t.Fatal("cancellation commit failure was ignored")
	}
	if candidate, claimed, err := newStore(coverageTx{row: coverageScanner(input), result: coverageRowsAffected(1)}).ClaimCancelling(ctx, build); err != nil || !claimed || candidate.AttemptID != "attempt" {
		t.Fatalf("valid cancellation = %#v claimed=%v err=%v", candidate, claimed, err)
	}
}

func TestRunReconciliationErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	for _, test := range []struct {
		name string
		run  func(*RunStore) error
		make func(coverageTx) coverageTx
	}{
		{name: "timed out begin", run: func(s *RunStore) error { return s.ReconcileTimedOutDispatches(ctx, now) }},
		{name: "stale begin", run: func(s *RunStore) error { return s.ReconcileStaleCancellations(ctx, now) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(&RunStore{pool: coverageDatabase{beginErr: errors.New("begin")}}); err == nil {
				t.Fatal("begin failure was ignored")
			}
		})
	}
	emptyRows := func(rows *coverageRows) *RunStore {
		return &RunStore{pool: coverageDatabase{tx: coverageTx{rows: rows}}}
	}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{queryErr: errors.New("query")}}}).ReconcileTimedOutDispatches(ctx, now); err == nil {
		t.Fatal("timed-out query failure was ignored")
	}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{queryErr: errors.New("query")}}}).ReconcileStaleCancellations(ctx, now); err == nil {
		t.Fatal("stale cancellation query failure was ignored")
	}
	badScan := &coverageRows{rows: []func(...any) error{func(...any) error { return errors.New("scan") }}}
	if err := emptyRows(badScan).ReconcileTimedOutDispatches(ctx, now); err == nil {
		t.Fatal("timed-out scan failure was ignored")
	}
	if err := emptyRows(&coverageRows{rows: []func(...any) error{func(...any) error { return errors.New("scan") }}}).ReconcileStaleCancellations(ctx, now); err == nil {
		t.Fatal("stale cancellation scan failure was ignored")
	}
	if err := emptyRows(&coverageRows{err: errors.New("rows")}).ReconcileTimedOutDispatches(ctx, now); err == nil {
		t.Fatal("timed-out rows failure was ignored")
	}
	if err := emptyRows(&coverageRows{err: errors.New("rows")}).ReconcileStaleCancellations(ctx, now); err == nil {
		t.Fatal("stale cancellation rows failure was ignored")
	}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{rows: &coverageRows{}, execErr: errors.New("exec")}}}).ReconcileTimedOutDispatches(ctx, now); err != nil {
		t.Fatalf("empty timed-out reconciliation failed: %v", err)
	}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{rows: &coverageRows{}, commitErr: errors.New("commit")}}}).ReconcileTimedOutDispatches(ctx, now); err == nil {
		t.Fatal("timed-out commit failure was ignored")
	}
	if err := (&RunStore{pool: coverageDatabase{tx: coverageTx{rows: &coverageRows{}, commitErr: errors.New("commit")}}}).ReconcileStaleCancellations(ctx, now); err == nil {
		t.Fatal("stale cancellation commit failure was ignored")
	}
}

func TestRunnerArchiveAndDeleteBranches(t *testing.T) {
	ctx := context.Background()
	testRunnerDeleteBranches(t, ctx)
	testRunnerArchiveBranches(t, ctx)
}

func testRunnerDeleteBranches(t *testing.T, ctx context.Context) {
	pgError := &pgconn.PgError{Code: "23503", Message: "history"}
	if deleted, err := (&RunnerStore{pool: coverageDatabase{result: coverageRowsAffected(0), err: pgError}}).Delete(ctx, "runner"); err != ErrRunnerHasExecutionHistory || deleted {
		t.Fatalf("history delete = deleted=%v err=%v", deleted, err)
	}
	if deleted, err := (&RunnerStore{pool: coverageDatabase{result: coverageRowsAffected(0), err: errors.New("delete")}}).Delete(ctx, "runner"); err == nil || deleted {
		t.Fatal("runner delete failure was ignored")
	}
}

func testRunnerArchiveBranches(t *testing.T, ctx context.Context) {
	newStore := func(tx coverageTx) *RunnerStore { return &RunnerStore{pool: coverageDatabase{tx: tx}} }
	if archived, err := (&RunnerStore{pool: coverageDatabase{beginErr: errors.New("begin")}}).Archive(ctx, "runner"); err == nil || archived {
		t.Fatal("archive begin failure was ignored")
	}
	if archived, err := newStore(coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows })}).Archive(ctx, "runner"); err != nil || archived {
		t.Fatalf("missing archive = archived=%v err=%v", archived, err)
	}
	if archived, err := newStore(coverageTx{row: coverageScanner(func(...any) error { return errors.New("scan") })}).Archive(ctx, "runner"); err == nil || archived {
		t.Fatal("archive scan failure was ignored")
	}
	if archived, err := newStore(coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*bool)) = true; return nil })}).Archive(ctx, "runner"); err != nil || !archived {
		t.Fatalf("already archived = archived=%v err=%v", archived, err)
	}
	if archived, err := newStore(coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*bool)) = false; return nil }), execErr: errors.New("update")}).Archive(ctx, "runner"); err == nil || archived {
		t.Fatal("archive update failure was ignored")
	}
	if archived, err := newStore(coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*bool)) = false; return nil }), commitErr: errors.New("commit")}).Archive(ctx, "runner"); err == nil || archived {
		t.Fatal("archive commit failure was ignored")
	}
}

func TestRunnerEnrollmentBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	validEnrollment := func(used *time.Time, expires time.Time) coverageScanner {
		return func(dest ...any) error {
			*(dest[0].(*string)) = "runner"
			*(dest[1].(*time.Time)) = expires
			*(dest[2].(**time.Time)) = used
			return nil
		}
	}
	for _, test := range []struct {
		name string
		row  coverageScanner
	}{
		{name: "missing", row: coverageScanner(func(...any) error { return pgx.ErrNoRows })},
		{name: "query failure", row: coverageScanner(func(...any) error { return errors.New("query") })},
		{name: "used", row: validEnrollment(&now, now.Add(time.Hour))},
		{name: "expired", row: validEnrollment(nil, now.Add(-time.Minute))},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&RunnerStore{pool: coverageDatabase{tx: coverageTx{row: test.row}}}).ConsumeEnrollment(ctx, "hash", now)
			if err == nil {
				t.Fatal("invalid enrollment was accepted")
			}
		})
	}
	publicKey := make([]byte, ed25519.PublicKeySize)
	newStore := func(tx coverageTx) *RunnerStore {
		return &RunnerStore{pool: coverageDatabase{tx: tx}}
	}
	if _, err := newStore(coverageTx{row: validEnrollment(nil, now.Add(time.Hour)), execErr: errors.New("mark used")}).ConsumeEnrollment(ctx, "hash", now); err == nil {
		t.Fatal("enrollment update failure was ignored")
	}
	keyRows := &coverageScanSequence{scans: []coverageScanner{
		validEnrollment(nil, now.Add(time.Hour)),
		func(dest ...any) error {
			*(dest[0].(*string)) = "other-runner"
			*(dest[1].(*[]byte)) = publicKey
			return nil
		},
	}}
	if _, err := newStore(coverageTx{row: keyRows}).ConsumeEnrollmentWithKey(ctx, "hash", now, "key", publicKey); err == nil {
		t.Fatal("enrollment key conflict was ignored")
	}
	keyQueryFailure := &coverageScanSequence{scans: []coverageScanner{
		validEnrollment(nil, now.Add(time.Hour)),
		func(...any) error { return errors.New("key query") },
	}}
	if _, err := newStore(coverageTx{row: keyQueryFailure}).ConsumeEnrollmentWithKey(ctx, "hash", now, "key", publicKey); err == nil {
		t.Fatal("enrollment key query failure was ignored")
	}
}

func TestRunnerHeartbeatBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	publicKey := make([]byte, ed25519.PublicKeySize)
	active := func(value bool) coverageScanner {
		return func(dest ...any) error { *(dest[0].(*bool)) = value; return nil }
	}
	key := func(runner string, value []byte) coverageScanner {
		return func(dest ...any) error {
			*(dest[0].(*string)) = runner
			*(dest[1].(*[]byte)) = value
			return nil
		}
	}
	newStore := func(tx coverageTx) *RunnerStore { return &RunnerStore{pool: coverageDatabase{tx: tx}} }
	input := heartbeatInput{runnerID: "runner", bootID: "boot", now: now, capacity: 2, keyID: "key", publicKey: publicKey}
	if err := newStore(coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows })}).heartbeatWithKeyAndCapacity(ctx, input); err == nil {
		t.Fatal("missing heartbeat runner was accepted")
	}
	if err := newStore(coverageTx{row: active(false)}).heartbeatWithKeyAndCapacity(ctx, input); err == nil {
		t.Fatal("archived runner heartbeat was accepted")
	}
	if err := newStore(coverageTx{row: active(true), execErr: errors.New("session")}).heartbeatWithKeyAndCapacity(ctx, input); err == nil {
		t.Fatal("heartbeat session failure was ignored")
	}
	missingKey := &coverageScanSequence{scans: []coverageScanner{active(true), func(...any) error { return pgx.ErrNoRows }}}
	if err := newStore(coverageTx{row: missingKey}).heartbeatWithKeyAndCapacity(ctx, input); err == nil {
		t.Fatal("missing heartbeat key was accepted")
	}
	mismatchedKey := &coverageScanSequence{scans: []coverageScanner{active(true), key("other", publicKey)}}
	if err := newStore(coverageTx{row: mismatchedKey}).heartbeatWithKeyAndCapacity(ctx, input); err == nil {
		t.Fatal("mismatched heartbeat key was accepted")
	}
	validKey := &coverageScanSequence{scans: []coverageScanner{active(true), key("runner", publicKey)}}
	sample := RunnerMetricsSample{CPUPercent: 10, MemoryPercent: 20, MemoryUsedBytes: 1, MemoryTotalBytes: 2}
	if err := newStore(coverageTx{row: validKey, result: coverageRowsAffected(1)}).heartbeatWithKeyAndCapacity(ctx, heartbeatInput{runnerID: "runner", bootID: "boot", now: now, capacity: 2, sample: &sample, keyID: "key", publicKey: publicKey}); err != nil {
		t.Fatalf("valid heartbeat failed: %v", err)
	}
	validKey = &coverageScanSequence{scans: []coverageScanner{active(true), key("runner", publicKey)}}
	if err := newStore(coverageTx{row: validKey, result: coverageRowsAffected(1), commitErr: errors.New("commit")}).heartbeatWithKeyAndCapacity(ctx, heartbeatInput{runnerID: "runner", bootID: "boot", now: now, keyID: "key", publicKey: publicKey}); err == nil {
		t.Fatal("heartbeat commit failure was ignored")
	}
}

func TestRetentionDeleteBranches(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Now().UTC()
	if _, err := deleteRunnerMetricsBatch(ctx, coverageTx{execErr: errors.New("metrics")}, cutoff, 1); err == nil {
		t.Fatal("metrics deletion failure was ignored")
	}
	if _, err := deleteRunBatch(ctx, coverageTx{execErr: errors.New("runs")}, cutoff, 1); err == nil {
		t.Fatal("run deletion failure was ignored")
	}
	if _, err := deleteDeadLetterBatch(ctx, coverageTx{execErr: errors.New("dead letters")}, cutoff, 1); err == nil {
		t.Fatal("dead-letter deletion failure was ignored")
	}
	if _, err := deleteAuditBatch(ctx, coverageTx{execErr: errors.New("audit")}, cutoff, 1); err == nil {
		t.Fatal("audit deletion failure was ignored")
	}
	if _, err := purgeRows(ctx, coverageTx{execErr: errors.New("runs")}, cutoff, cutoff, cutoff, 1, false); err == nil {
		t.Fatal("retention purge failure was ignored")
	}
	if result, err := purgeRows(ctx, coverageTx{result: coverageRowsAffected(3)}, cutoff, cutoff, cutoff, 1, true); err != nil || result.Runs != 3 {
		t.Fatalf("critical purge = %#v err=%v", result, err)
	}
}

func TestRunClaimWaitingBranches(t *testing.T) {
	ctx := context.Background()
	scanCandidate := coverageScanner(func(dest ...any) error {
		*(dest[0].(*string)) = "run"
		*(dest[1].(*string)) = "task"
		*(dest[2].(*string)) = "task-v1"
		*(dest[3].(*string)) = "Task"
		*(dest[4].(*int)) = 1
		*(dest[5].(*[]byte)) = []byte(`["echo", "hello"]`)
		*(dest[6].(*string)) = "."
		*(dest[7].(*int)) = 30
		*(dest[8].(*int)) = 1024
		*(dest[9].(*string)) = "digest"
		*(dest[10].(*[]byte)) = []byte(`{}`)
		*(dest[11].(*[]byte)) = []byte(`{}`)
		*(dest[12].(*[]byte)) = []byte(`{}`)
		*(dest[13].(*[]byte)) = []byte(`{}`)
		*(dest[14].(*[]byte)) = []byte(`[]`)
		*(dest[15].(*string)) = "pool"
		*(dest[16].(*string)) = "session"
		*(dest[17].(*string)) = "runner"
		*(dest[18].(*int)) = 1
		return nil
	})
	newStore := func(tx coverageTx) *RunStore { return &RunStore{pool: coverageDatabase{tx: tx}} }
	build := func(candidate DispatchCandidate) ([]byte, error) {
		if candidate.RunID != "run" || candidate.AttemptNumber != 1 {
			return nil, errors.New("candidate was not normalized")
		}
		return []byte("order"), nil
	}
	if candidate, claimed, err := newStore(coverageTx{row: scanCandidate, result: coverageRowsAffected(1)}).ClaimWaiting(ctx, build); err != nil || !claimed || candidate.AttemptID != "run-attempt-1" {
		t.Fatalf("valid dispatch = %#v claimed=%v err=%v", candidate, claimed, err)
	}
	if _, claimed, err := newStore(coverageTx{row: scanCandidate}).ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return nil, errors.New("build") }); err == nil || claimed {
		t.Fatal("dispatch build failure was ignored")
	}
	if _, claimed, err := newStore(coverageTx{row: scanCandidate}).ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return nil, nil }); err == nil || claimed {
		t.Fatal("empty dispatch order was accepted")
	}
	if _, claimed, err := newStore(coverageTx{row: scanCandidate, execErr: errors.New("insert")}).ClaimWaiting(ctx, build); err == nil || claimed {
		t.Fatal("dispatch insert failure was ignored")
	}
	if _, claimed, err := newStore(coverageTx{row: scanCandidate, result: coverageRowsAffected(1), commitErr: errors.New("commit")}).ClaimWaiting(ctx, build); err == nil || claimed {
		t.Fatal("dispatch commit failure was ignored")
	}
	invalidCommand := coverageScanner(func(dest ...any) error {
		if err := scanCandidate(dest...); err != nil {
			return err
		}
		*(dest[5].(*[]byte)) = []byte("invalid")
		return nil
	})
	if _, claimed, err := newStore(coverageTx{row: invalidCommand}).ClaimWaiting(ctx, build); err == nil || claimed {
		t.Fatal("invalid dispatch command was accepted")
	}
}

func TestStoreListScanErrors(t *testing.T) {
	ctx := context.Background()
	badRows := func() *coverageRows {
		return &coverageRows{rows: []func(...any) error{func(...any) error { return errors.New("scan") }}}
	}
	zeroCount := coverageScanner(func(dest ...any) error {
		switch value := dest[0].(type) {
		case *int:
			*value = 0
		case *int64:
			*value = 0
		}
		return nil
	})
	if _, _, err := NewAuditRepository(coverageDatabase{
		row: coverageScanner(func(dest ...any) error {
			*(dest[0].(*int)) = 0
			*(dest[1].(*int)) = 0
			*(dest[2].(*int)) = 0
			return nil
		}),
		rows: badRows(),
	}).Query(ctx, AuditFilter{}); err == nil {
		t.Fatal("audit row scan failure was ignored")
	}
	tests := []struct {
		name string
		call func(database) error
	}{
		{name: "dead letters", call: func(db database) error {
			_, _, err := NewDeadLetterRepository(db, []byte("secret")).List(ctx, DeadLetterFilter{})
			return err
		}},
		{name: "encrypted statuses", call: func(db database) error { _, err := NewEncryptedSecretRepository(db).ListStatuses(ctx); return err }},
		{name: "exit codes", call: func(db database) error { _, err := NewExitCodeRepository(db).List(ctx); return err }},
		{name: "resources", call: func(db database) error { _, err := NewResourceRepository(db).List(ctx); return err }},
		{name: "roles", call: func(db database) error { _, err := NewRoleRepository(db).List(ctx); return err }},
		{name: "runner pools", call: func(db database) error { _, err := NewRunnerRepository(db).ListPools(ctx); return err }},
		{name: "runners", call: func(db database) error { _, err := NewRunnerRepository(db).List(ctx); return err }},
		{name: "archived runners", call: func(db database) error { _, err := NewRunnerRepository(db).ListArchived(ctx); return err }},
		{name: "sessions", call: func(db database) error { _, err := NewSessionRepository(db).List(ctx, "user"); return err }},
		{name: "admin sessions", call: func(db database) error {
			_, _, err := NewSessionRepository(db).ListAdminPage(ctx, "", 1, 0)
			return err
		}},
		{name: "oidc providers", call: func(db database) error { _, err := NewOIDCProviderRepository(db).List(ctx); return err }},
		{name: "identities", call: func(db database) error {
			_, err := NewOIDCProviderRepository(db).ListIdentities(ctx, "user")
			return err
		}},
		{name: "group mappings", call: func(db database) error {
			_, err := NewOIDCProviderRepository(db).ListGroupRoleMappings(ctx, "provider")
			return err
		}},
		{name: "schedules", call: func(db database) error { _, err := NewScheduleRepository(db).List(ctx); return err }},
		{name: "schedule projection", call: func(db database) error { _, err := NewScheduleRepository(db).ListScheduleProjection(ctx); return err }},
		{name: "tasks", call: func(db database) error { _, err := NewTaskRepository(db).List(ctx, false); return err }},
		{name: "task versions", call: func(db database) error { _, err := NewTaskRepository(db).ListVersions(ctx, "task"); return err }},
		{name: "pending dispatch", call: func(db database) error { _, err := NewRunRepository(db).PendingDispatch(ctx, 1); return err }},
		{name: "log chunks", call: func(db database) error {
			_, err := NewRunRepository(db).ListLogChunks(ctx, "run", "stdout", 0)
			return err
		}},
		{name: "users", call: func(db database) error { _, err := NewUserRepository(db).List(ctx, ""); return err }},
		{name: "user page", call: func(db database) error {
			_, _, err := NewUserRepository(db).ListPage(ctx, "", "", nil, 1, 1)
			return err
		}},
		{name: "runner metrics", call: func(db database) error {
			_, err := NewRunnerRepository(db).ListRunnerMetrics(ctx, "runner", time.Now().Add(-time.Hour), time.Now(), 1)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(coverageDatabase{row: zeroCount, rows: badRows()}); err == nil {
				t.Fatal("row scan failure was ignored")
			}
		})
	}
}

func TestSQLiteConversionErrors(t *testing.T) {
	var boolean bool
	if err := assignSQLite(&boolean, "not-a-bool"); err == nil {
		t.Fatal("invalid SQLite boolean was accepted")
	}
	var number int
	if err := assignSQLite(&number, "not-a-number"); err == nil {
		t.Fatal("invalid SQLite integer was accepted")
	}
	var decimal float64
	if err := assignSQLite(&decimal, "not-a-decimal"); err == nil {
		t.Fatal("invalid SQLite float was accepted")
	}
	if err := decodeSQLiteArguments([]any{123}, "decode($1, 'hex')"); err == nil {
		t.Fatal("non-text SQLite decode argument was accepted")
	}
	if err := decodeSQLiteArguments([]any{"not-hex"}, "decode($1, 'hex')"); err == nil {
		t.Fatal("invalid SQLite hex argument was accepted")
	}
	if _, err := databaseFrom(struct{}{}); err == nil {
		t.Fatal("unsupported database was accepted")
	}
}

func TestSQLiteSSOProviderAndMappingLifecycle(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	providers := NewOIDCProviderRepository(db)
	provider := OIDCProviderRecord{ID: "provider-lifecycle", Name: "Lifecycle", Issuer: "https://issuer.example", ClientID: "client", CallbackURLs: []string{"https://app.example/callback"}, Enabled: true, AutoProvision: true}
	if err := providers.Upsert(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if items, err := providers.List(ctx); err != nil || len(items) != 1 || len(items[0].CallbackURLs) != 1 {
		t.Fatalf("providers = %#v, err=%v", items, err)
	}
	if got, found, err := providers.Find(ctx, provider.ID); err != nil || !found || got.Issuer != provider.Issuer {
		t.Fatalf("provider = %#v found=%v err=%v", got, found, err)
	}
	if _, found, err := providers.Find(ctx, "missing-provider"); err != nil || found {
		t.Fatalf("missing provider = found=%v err=%v", found, err)
	}
	if count, err := providers.EnabledCount(ctx); err != nil || count != 1 {
		t.Fatalf("enabled providers = %d err=%v", count, err)
	}
	if err := NewRoleRepository(db).Ensure(ctx, "sso-role-lifecycle", "SSO lifecycle", "", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := NewUserRepository(db).Create(ctx, UserRecord{ID: "sso-user-lifecycle", Username: "sso-user", Email: "sso-user@example.com", Enabled: true}, ""); err != nil {
		t.Fatal(err)
	}
	identity := SSOIdentityRecord{ID: "identity-lifecycle", UserID: "sso-user-lifecycle", ProviderID: provider.ID, Subject: "subject"}
	if err := providers.CreateIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if items, err := providers.ListIdentities(ctx, identity.UserID); err != nil || len(items) != 1 || items[0].Subject != identity.Subject {
		t.Fatalf("identities = %#v err=%v", items, err)
	}
	if got, found, err := providers.FindIdentity(ctx, identity.ProviderID, identity.Subject); err != nil || !found || got.ID != identity.ID {
		t.Fatalf("identity = %#v found=%v err=%v", got, found, err)
	}
	mapping := SSOGroupRoleMappingRecord{ProviderID: provider.ID, GroupName: "admins", RoleID: "sso-role-lifecycle"}
	if err := providers.SetGroupRoleMapping(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceGroupRoleMappings(ctx, provider.ID, []SSOGroupRoleMappingRecord{{GroupName: "operators", RoleID: mapping.RoleID}, {}, mapping}); err != nil {
		t.Fatal(err)
	}
	if items, err := providers.ListGroupRoleMappings(ctx, provider.ID); err != nil || len(items) != 2 {
		t.Fatalf("mappings = %#v err=%v", items, err)
	}
	if err := providers.DeleteGroupRoleMapping(ctx, provider.ID, "operators", mapping.RoleID); err != nil {
		t.Fatal(err)
	}
	if err := providers.DeleteGroupRoleMapping(ctx, provider.ID, "operators", mapping.RoleID); err == nil {
		t.Fatal("missing group mapping deletion succeeded")
	}
	if err := providers.DeleteIdentity(ctx, identity.UserID, identity.ProviderID, identity.Subject); err != nil {
		t.Fatal(err)
	}
	if err := providers.DeleteIdentity(ctx, identity.UserID, identity.ProviderID, identity.Subject); err == nil {
		t.Fatal("missing identity deletion succeeded")
	}
}

func TestTaskSerializationAndReferenceBranches(t *testing.T) {
	makeTaskScan := func(invalid int) coverageScanner {
		return func(dest ...any) error {
			for index, value := range dest {
				switch value := value.(type) {
				case *string:
					*value = "value"
				case *bool:
					*value = true
				case *int:
					*value = 1
				case *int64:
					*value = 1
				case *[]byte:
					*value = []byte(`{}`)
				case **int:
					*value = nil
				case *time.Time:
					*value = time.Now().UTC()
				}
				if index == invalid {
					if value, ok := value.(*[]byte); ok {
						*value = []byte("invalid")
					}
				}
			}
			return nil
		}
	}
	for _, index := range []int{7, 9, 10, 11, 17} {
		if _, err := scanTask(makeTaskScan(index)); err == nil {
			t.Fatalf("invalid task JSON at scan index %d was accepted", index)
		}
	}
	ctx := context.Background()
	if err := recordGlobalVariableReferences(ctx, coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*string)) = "variable"; return nil }), result: coverageRowsAffected(1)}, "task_version", "version", "$ENV:MODE"); err != nil {
		t.Fatalf("global variable reference failed: %v", err)
	}
	if err := recordGlobalVariableReferences(ctx, coverageTx{row: coverageScanner(func(...any) error { return pgx.ErrNoRows })}, "task_version", "version", "$ENV:MODE"); err == nil {
		t.Fatal("undefined global variable was accepted")
	}
	if err := recordGlobalVariableReferences(ctx, coverageTx{row: coverageScanner(func(...any) error { return errors.New("lookup") })}, "task_version", "version", "$ENV:MODE"); err == nil {
		t.Fatal("global variable lookup failure was ignored")
	}
	if err := recordGlobalVariableReferences(ctx, coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*string)) = "variable"; return nil }), execErr: errors.New("reference")}, "task_version", "version", "$ENV:MODE"); err == nil {
		t.Fatal("global variable reference insert failure was ignored")
	}
	if err := insertTaskVersion(ctx, coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*bool)) = false; return nil })}, "task", 1, TaskDefinition{RunnerPoolID: "pool"}); err == nil {
		t.Fatal("missing runner pool was accepted")
	}
	if err := insertTaskVersion(ctx, coverageTx{row: coverageScanner(func(...any) error { return errors.New("pool lookup") })}, "task", 1, TaskDefinition{RunnerPoolID: "pool"}); err == nil {
		t.Fatal("runner pool lookup failure was ignored")
	}
	if err := insertTaskVersion(ctx, coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*bool)) = true; return nil }), execErr: errors.New("version insert")}, "task", 1, TaskDefinition{RunnerPoolID: "pool"}); err == nil {
		t.Fatal("task version insert failure was ignored")
	}
}

func TestScheduleOccurrenceBranches(t *testing.T) {
	now := time.Now().UTC()
	base := DueScheduleRecord{Expression: "* * * * *", Timezone: "UTC", NextFireAt: now.Add(-2 * time.Minute), CatchupLimit: 2}
	next := func(due DueScheduleRecord) (time.Time, error) { return due.NextFireAt.Add(time.Minute), nil }
	for _, test := range []struct {
		name   string
		policy string
		limit  int
		want   time.Time
	}{
		{name: "skip", policy: "SKIP_ALL", want: time.Time{}},
		{name: "latest", policy: "RUN_LATEST", want: now.Add(-time.Minute)},
		{name: "bounded", policy: "RUN_UP_TO_N", limit: 2, want: now.Add(-time.Minute)},
		{name: "default", policy: "other", want: now.Add(-2 * time.Minute)},
	} {
		t.Run(test.name, func(t *testing.T) {
			due := base
			due.MisfirePolicy, due.CatchupLimit = test.policy, test.limit
			occurrence, _, err := chooseDueOccurrence(due, now, due.NextFireAt.Add(time.Minute), next)
			if err != nil || (test.want.IsZero() != occurrence.IsZero()) {
				t.Fatalf("occurrence=%v err=%v", occurrence, err)
			}
		})
	}
	if _, _, err := chooseDueOccurrence(base, now, base.NextFireAt, func(DueScheduleRecord) (time.Time, error) { return time.Time{}, errors.New("next") }); err == nil {
		t.Fatal("next-fire failure was ignored")
	}
	if _, _, err := chooseDueOccurrence(base, now, base.NextFireAt, func(DueScheduleRecord) (time.Time, error) { return time.Time{}, nil }); err == nil {
		t.Fatal("empty next-fire value was accepted")
	}
}

func TestRunEventValidationAndOrderingBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	scanAttempt := func(state string, lastSequence int64) coverageScanner {
		return func(dest ...any) error {
			*(dest[0].(*string)) = "attempt"
			*(dest[1].(*string)) = "runner"
			*(dest[2].(*string)) = "session"
			*(dest[3].(*string)) = "lease"
			*(dest[4].(*int64)) = 2
			*(dest[5].(*string)) = state
			*(dest[6].(*int64)) = lastSequence
			return nil
		}
	}
	base := RunEventInput{EventID: "event", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", FencingToken: 2, Attempt: 1, Sequence: 1, EventType: "accepted", Envelope: []byte("signed"), ReportedAt: now}
	newStore := func(tx databaseTx) *RunStore { return &RunStore{pool: coverageDatabase{tx: tx}} }
	if err := (&RunStore{pool: coverageDatabase{beginErr: errors.New("begin")}}).ApplyRunEvent(ctx, base); err == nil {
		t.Fatal("run event begin failure was ignored")
	}
	if err := newStore(coverageTx{row: coverageScanner(func(...any) error { return errors.New("attempt lookup") })}).ApplyRunEvent(ctx, base); err == nil {
		t.Fatal("run event lookup failure was ignored")
	}
	wrongSession := base
	wrongSession.RunnerSessionID = "other-session"
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 0)}).ApplyRunEvent(ctx, wrongSession); err == nil {
		t.Fatal("mismatched run event session was accepted")
	}
	wrongFence := base
	wrongFence.FencingToken = 3
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 0)}).ApplyRunEvent(ctx, wrongFence); err == nil {
		t.Fatal("mismatched run event fence was accepted")
	}
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 0), execErr: errors.New("inbox")}).ApplyRunEvent(ctx, base); err == nil {
		t.Fatal("event inbox failure was ignored")
	}
	duplicate := base
	duplicate.EventID = "duplicate"
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 0), result: coverageRowsAffected(0)}).ApplyRunEvent(ctx, duplicate); err != nil {
		t.Fatalf("duplicate event failed: %v", err)
	}
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 2), result: coverageRowsAffected(1)}).ApplyRunEvent(ctx, base); err != nil {
		t.Fatalf("stale event failed: %v", err)
	}
	illegal := base
	illegal.EventID, illegal.EventType, illegal.Sequence = "illegal", "completed", 3
	if err := newStore(coverageTx{row: scanAttempt("DISPATCHED", 0), result: coverageRowsAffected(1)}).ApplyRunEvent(ctx, illegal); err == nil {
		t.Fatal("illegal run event transition was accepted")
	}
	logEvent := base
	logEvent.EventID, logEvent.EventType, logEvent.EventChannel, logEvent.Result = "log", "log_chunk", "stdout", "output"
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 0), result: coverageRowsAffected(1)}).ApplyRunEvent(ctx, logEvent); err != nil {
		t.Fatalf("log event failed: %v", err)
	}
	if err := newStore(coverageTx{row: scanAttempt("ACCEPTED", 0), result: coverageRowsAffected(0), commitErr: errors.New("commit")}).ApplyRunEvent(ctx, duplicate); err == nil {
		t.Fatal("duplicate event commit failure was ignored")
	}
	if err := startRunAttempt(ctx, &coverageExecSequence{steps: []coverageExecStep{{result: coverageRowsAffected(1)}, {err: errors.New("run update")}}}, base, "attempt"); err == nil {
		t.Fatal("run transition update failure was ignored")
	}
	if err := releaseRunResources(ctx, &coverageExecSequence{steps: []coverageExecStep{{err: errors.New("runner")}}}, "runner", "attempt"); err == nil {
		t.Fatal("runner resource release failure was ignored")
	}
	if err := (&RunStore{}).applyUnknownRunEvent(ctx, coverageTx{execErr: errors.New("attempt")}, base, "attempt", "runner"); err == nil {
		t.Fatal("unknown event update failure was ignored")
	}
}

func TestTransactionalRepositoryValidationBranches(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("storage")
	validUser := UserRecord{ID: "user", Username: "user", Email: "user@example.com", Enabled: true}
	if err := (&UserStore{}).ProvisionLocal(ctx, UserRecord{Status: "invalid"}, "", "role", ""); err == nil {
		t.Fatal("invalid local user status was accepted")
	}
	if err := (&UserStore{pool: coverageDatabase{beginErr: failure}}).ProvisionLocal(ctx, validUser, "", "role", ""); err == nil {
		t.Fatal("local provisioning begin failure was ignored")
	}
	if err := (&UserStore{pool: coverageDatabase{tx: coverageTx{execErr: failure}}}).ProvisionLocal(ctx, validUser, "", "role", ""); err == nil {
		t.Fatal("local provisioning insert failure was ignored")
	}
	if err := (&UserStore{pool: coverageDatabase{tx: coverageTx{commitErr: failure}}}).ProvisionLocal(ctx, validUser, "", "role", ""); err == nil {
		t.Fatal("local provisioning commit failure was ignored")
	}
	if err := (&OIDCProviderStore{pool: coverageDatabase{tx: coverageTx{}}}).ProvisionOIDC(ctx, UserRecord{Status: "invalid"}, "role", "", SSOIdentityRecord{}); err == nil {
		t.Fatal("invalid OIDC user status was accepted")
	}
	if err := (&OIDCProviderStore{pool: coverageDatabase{beginErr: failure}}).ProvisionOIDC(ctx, validUser, "role", "", SSOIdentityRecord{}); err == nil {
		t.Fatal("OIDC provisioning begin failure was ignored")
	}
	if err := (&OIDCProviderStore{pool: coverageDatabase{tx: coverageTx{execErr: failure}}}).ProvisionOIDC(ctx, validUser, "role", "", SSOIdentityRecord{}); err == nil {
		t.Fatal("OIDC provisioning insert failure was ignored")
	}
	if err := (&OIDCProviderStore{pool: coverageDatabase{tx: coverageTx{commitErr: failure}}}).ProvisionOIDC(ctx, validUser, "role", "", SSOIdentityRecord{}); err == nil {
		t.Fatal("OIDC provisioning commit failure was ignored")
	}

	taskDefinition := TaskDefinition{ID: "task", Name: "Task", RunnerPoolID: "pool", Command: []string{"true"}}
	if _, err := (&TaskStore{pool: coverageDatabase{beginErr: failure}}).Create(ctx, taskDefinition); err == nil {
		t.Fatal("task create begin failure was ignored")
	}
	if _, err := (&TaskStore{pool: coverageDatabase{tx: coverageTx{execErr: failure}}}).Create(ctx, taskDefinition); err == nil {
		t.Fatal("task create insert failure was ignored")
	}
	if _, err := (&TaskStore{pool: coverageDatabase{tx: coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*bool)) = true; return nil }), commitErr: failure}}}).Create(ctx, taskDefinition); err == nil {
		t.Fatal("task create commit failure was ignored")
	}
	scheduleDefinition := ScheduleDefinition{ID: "schedule", Name: "Schedule", TaskID: "task", Expression: "* * * * *"}
	if _, err := (&ScheduleStore{pool: coverageDatabase{beginErr: failure}}).Create(ctx, scheduleDefinition); err == nil {
		t.Fatal("schedule create begin failure was ignored")
	}
	if _, err := (&ScheduleStore{pool: coverageDatabase{tx: coverageTx{row: coverageScanner(func(...any) error { return failure })}}}).Create(ctx, scheduleDefinition); err == nil {
		t.Fatal("schedule create task lookup failure was ignored")
	}
	if _, err := (&ScheduleStore{pool: coverageDatabase{tx: coverageTx{row: coverageScanner(func(dest ...any) error { *(dest[0].(*string)) = "task-v1"; return nil }), execErr: failure}}}).Create(ctx, scheduleDefinition); err == nil {
		t.Fatal("schedule create insert failure was ignored")
	}
	if _, err := (&ScheduleStore{pool: coverageDatabase{beginErr: failure}}).Update(ctx, "schedule", scheduleDefinition); err == nil {
		t.Fatal("schedule update begin failure was ignored")
	}
}

func stringPtr(value string) *string { return &value }
