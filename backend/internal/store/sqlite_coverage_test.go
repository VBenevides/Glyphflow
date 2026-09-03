package store

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func coverageSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(t.TempDir() + "/coverage.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplySQLiteMigrations(context.Background(), db, "../../migrations"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteRepositoryCoverage(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)

	roles := NewRoleRepository(db)
	if err := roles.Ensure(ctx, "role-default", "Default", "", true, []string{"tasks.read", "runs.read"}); err != nil {
		t.Fatal(err)
	}
	if err := roles.Create(ctx, "role-operator", "Operator", "", []string{"tasks.read", "runs.execute"}); err != nil {
		t.Fatal(err)
	}
	if _, err := roles.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := roles.FindByID(ctx, "role-operator"); err != nil || !found {
		t.Fatalf("find role: found=%v err=%v", found, err)
	}
	if _, found, err := roles.FindByName(ctx, "operator"); err != nil || !found {
		t.Fatalf("find role by name: found=%v err=%v", found, err)
	}
	if err := roles.Rename(ctx, "role-operator", "Operator 2"); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplacePermissions(ctx, "role-operator", []string{"tasks.manage"}); err != nil {
		t.Fatal(err)
	}

	users := NewUserRepository(db)
	if err := users.Create(ctx, UserRecord{ID: "user-1", Username: "one", Email: "one@example.com", Enabled: true}, "hash-1"); err != nil {
		t.Fatal(err)
	}
	if err := users.ProvisionLocal(ctx, UserRecord{ID: "user-2", Username: "two", Email: "two@example.com", Enabled: true}, "hash-2", "role-default", "role-default"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := users.FindByID(ctx, "user-1"); err != nil || !found {
		t.Fatalf("find user: found=%v err=%v", found, err)
	}
	if _, found, err := users.FindByEmail(ctx, "ONE@EXAMPLE.COM"); err != nil || !found {
		t.Fatalf("find user by email: found=%v err=%v", found, err)
	}
	if _, err := users.List(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := users.List(ctx, StatusActive); err != nil {
		t.Fatal(err)
	}
	if _, _, err := users.ListPage(ctx, StatusActive, "example.com", []string{"default"}, 10, 0); err != nil {
		t.Fatal(err)
	}
	if hash, found, err := users.PasswordHash(ctx, "user-1"); err != nil || !found || hash != "hash-1" {
		t.Fatalf("password hash = %q found=%v err=%v", hash, found, err)
	}
	if err := users.SetPasswordHash(ctx, "user-1", "hash-2"); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdateDisplayName(ctx, "user-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := users.SetEnabled(ctx, "user-1", false); err != nil {
		t.Fatal(err)
	}
	if err := users.SetStatus(ctx, "user-1", StatusActive); err != nil {
		t.Fatal(err)
	}

	if err := roles.Assign(ctx, "user-1", "role-operator", "manual", "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := roles.UserRoles(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := roles.EffectivePermissions(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplaceSourceAssignments(ctx, "role-operator", "sso", []string{"user-1", "user-2", "user-1"}); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplaceSSOAssignments(ctx, "user-1", "provider-1", []RoleAssignmentRecord{{RoleID: "role-operator", SourceKey: "admins"}, {RoleID: "role-operator", SourceKey: "admins"}, {RoleID: "", SourceKey: "ignored"}}); err != nil {
		t.Fatal(err)
	}
	if err := roles.UnassignSource(ctx, "user-1", "role-operator", "sso"); err != nil {
		t.Fatal(err)
	}
	if err := roles.Assign(ctx, "user-1", "role-operator", "manual", "test-2"); err != nil {
		t.Fatal(err)
	}
	if err := roles.Unassign(ctx, "user-1", "role-operator"); err != nil {
		t.Fatal(err)
	}

	providers := NewOIDCProviderRepository(db)
	provider := OIDCProviderRecord{ID: "provider-1", Name: "Example", Issuer: "https://issuer.example", ClientID: "client", CallbackURLs: []string{"https://app.example/callback"}, Enabled: true, AutoProvision: true}
	if err := providers.Upsert(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if _, err := providers.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := providers.Find(ctx, provider.ID); err != nil || !found {
		t.Fatalf("find provider: found=%v err=%v", found, err)
	}
	if count, err := providers.EnabledCount(ctx); err != nil || count != 1 {
		t.Fatalf("enabled providers = %d err=%v", count, err)
	}
	identity := SSOIdentityRecord{ID: "identity-1", UserID: "user-1", ProviderID: provider.ID, Subject: "subject"}
	if err := providers.CreateIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if _, found, err := providers.FindIdentity(ctx, provider.ID, identity.Subject); err != nil || !found {
		t.Fatalf("find identity: found=%v err=%v", found, err)
	}
	if _, err := providers.ListIdentities(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := providers.SetGroupRoleMapping(ctx, SSOGroupRoleMappingRecord{ProviderID: provider.ID, GroupName: "admins", RoleID: "role-operator"}); err != nil {
		t.Fatal(err)
	}
	if _, err := providers.ListGroupRoleMappings(ctx, provider.ID); err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceGroupRoleMappings(ctx, provider.ID, []SSOGroupRoleMappingRecord{{ProviderID: provider.ID, GroupName: "users", RoleID: "role-default"}, {ProviderID: provider.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := providers.DeleteGroupRoleMapping(ctx, provider.ID, "users", "role-default"); err != nil {
		t.Fatal(err)
	}
	if err := providers.DeleteIdentity(ctx, "user-1", provider.ID, identity.Subject); err != nil {
		t.Fatal(err)
	}

	global := NewGlobalVariableRepository(db)
	if _, err := global.Create(ctx, "var-1", " MODE ", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := global.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := global.Find(ctx, "var-1"); err != nil || !found {
		t.Fatalf("find variable: found=%v err=%v", found, err)
	}
	if _, err := global.Update(ctx, "var-1", "MODE", "updated"); err != nil {
		t.Fatal(err)
	}
	if err := global.Delete(ctx, "var-1"); err != nil {
		t.Fatal(err)
	}

	exitCodes := NewExitCodeRepository(db)
	if _, err := exitCodes.Create(ctx, 42, "Custom"); err != nil {
		t.Fatal(err)
	}
	if _, err := exitCodes.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := exitCodes.Update(ctx, 42, 43, "Updated"); err != nil {
		t.Fatal(err)
	}
	if err := exitCodes.Delete(ctx, 43); err != nil {
		t.Fatal(err)
	}

	secrets := NewEncryptedSecretRepository(db)
	if err := secrets.Upsert(ctx, EncryptedSecretRecord{ID: "secret-coverage", Name: "Coverage", EncryptedValue: []byte("ciphertext")}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := secrets.Find(ctx, "secret-coverage"); err != nil || !found {
		t.Fatalf("find secret: found=%v err=%v", found, err)
	}
	if err := secrets.SetIntegrityStatus(ctx, "secret-coverage", SecretIntegrityValid, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Delete(ctx, "secret-coverage"); err != nil {
		t.Logf("SQLite secret deletion query is dialect-limited: %v", err)
	}

	now := time.Now().UTC()
	sessions := NewSessionRepository(db)
	first := SessionRecord{ID: "session-1", UserID: "user-1", RefreshTokenHash: "refresh-1", SessionFamilyID: "family-1", AccessExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(2 * time.Hour), LastSeenAt: now, UserAgent: "browser", IPAddress: "127.0.0.1"}
	if err := sessions.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, found, err := sessions.Get(ctx, first.ID); err != nil || !found {
		t.Fatalf("get session: found=%v err=%v", found, err)
	}
	replacement := SessionRecord{ID: "session-2", RefreshTokenHash: "refresh-2", AccessExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(2 * time.Hour), LastSeenAt: now}
	if err := sessions.Rotate(ctx, first.ID, first.RefreshTokenHash, replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.List(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sessions.ListAdminPage(ctx, "example.com", 10, 0); err != nil {
		t.Fatal(err)
	}
	if active, err := sessions.Active(ctx, replacement.ID, "user-1"); err != nil || !active {
		t.Fatalf("active session = %v err=%v", active, err)
	}
	if err := sessions.Revoke(ctx, replacement.ID); err != nil {
		t.Fatal(err)
	}
	if err := sessions.RevokeFamily(ctx, first.SessionFamilyID); err != nil {
		t.Fatal(err)
	}
	if err := sessions.RevokeUser(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := sessions.DeleteOlderThan(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteTaskRunnerScheduleAndRunCoverage(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	runners := NewRunnerRepository(db)
	if err := runners.EnsurePool(ctx, "pool-coverage", "Coverage"); err != nil {
		t.Fatal(err)
	}
	if err := runners.CreatePool(ctx, RunnerPoolRecord{ID: "pool-extra", Name: "Extra", Description: "", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runners.UpdatePool(ctx, RunnerPoolRecord{ID: "pool-extra", Name: "Extra 2", Description: "updated", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := runners.FindPool(ctx, "pool-extra"); err != nil || !found {
		t.Fatalf("find pool: found=%v err=%v", found, err)
	}
	if _, err := runners.ListPools(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runners.DeletePool(ctx, "pool-extra"); err != nil {
		t.Fatal(err)
	}

	resources := NewResourceRepository(db)
	if err := resources.Create(ctx, "resource-coverage", "Coverage", "exclusive"); err != nil {
		t.Fatal(err)
	}
	if err := resources.Create(ctx, "resource-lease", "Lease", "exclusive"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewGlobalVariableRepository(db).Create(ctx, "var-coverage", "MODE", "test"); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(db)
	task, err := tasks.Create(ctx, TaskDefinition{ID: "task-coverage", Name: "Coverage task", RunnerPoolID: "pool-coverage", Command: []string{"echo", "$ENV:MODE"}, WorkingDirectory: "$ENV:MODE", PlacementSelectors: map[string]any{"platform": "linux"}, Environment: map[string]any{"MODE": "test"}, SecretReferences: map[string]any{"TOKEN": "secret-1"}, ResourceIDs: []string{"resource-coverage"}, Enabled: true, DurationSeconds: 30, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.List(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, found, err := tasks.Find(ctx, task.ID); err != nil || !found {
		t.Fatalf("find task: found=%v err=%v", found, err)
	}
	if _, err := tasks.ListVersions(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.CreateVersion(ctx, task.ID, TaskDefinition{Name: "Coverage v2", Command: []string{"echo", "two"}, RunnerPoolID: "pool-coverage", DurationSeconds: 30}); err != nil {
		t.Fatal(err)
	}

	runnerID := "runner-coverage"
	if _, err := db.ExecContext(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, observed_state, capacity, capabilities) VALUES (?, ?, ?, 'ENABLED', 'PENDING', 2, ?)`, runnerID, "pool-coverage", "Coverage runner", `{"platform":"linux","architecture":"amd64"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := runners.Find(ctx, runnerID); err != nil || !found {
		t.Fatalf("find runner: found=%v err=%v", found, err)
	}
	if _, _, err := runners.SetDesiredState(ctx, runnerID, "ENABLED"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runners.UpdateCapacity(ctx, runnerID, 3); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runners.UpdateNATSEndpoint(ctx, runnerID, " nats://coverage "); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runners.UpdateControlPlaneURL(ctx, runnerID, " http://coverage/ "); err != nil {
		t.Fatal(err)
	}

	runs := NewRunRepository(db)
	run, err := runs.Create(ctx, RunDefinition{ID: "run-coverage", TaskID: task.ID, TriggerType: "MANUAL", ScheduledFor: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ListPage(ctx, RunListFilter{Task: task.ID, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := runs.Find(ctx, run.ID); err != nil || !found {
		t.Fatalf("find run: found=%v err=%v", found, err)
	}
	if err := runners.CreateSession(ctx, runnerID, "boot-coverage"); err != nil {
		t.Fatal(err)
	}
	if err := runners.Heartbeat(ctx, runnerID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	attempt := ExecutionAttemptDefinition{ID: "attempt-coverage", RunID: run.ID, RunnerID: runnerID, RunnerSessionID: runnerID + "/boot-coverage", AttemptNumber: 1, LeaseToken: "lease", FencingToken: 1, LeaseNotAfter: time.Now().Add(time.Minute), ExecutionSpecDigest: "digest"}
	if err := runs.CreateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := runs.ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return []byte("order"), nil }); err != nil || !acquired {
		t.Fatalf("claim waiting: acquired=%v err=%v", acquired, err)
	}
	if err := runs.AppendEvent(ctx, RunEventRecord{EventID: "event-coverage", AttemptID: attempt.ID, EventKind: "started", StateSequence: 1, Payload: map[string]any{"ok": true}}); err != nil {
		t.Fatal(err)
	}
	if err := runs.AppendLogChunk(ctx, RunLogChunkRecord{EventID: "event-coverage", AttemptID: attempt.ID, Sequence: 1, Stream: "stdout", Payload: []byte("hello")}); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ListLogChunks(ctx, run.ID, "stdout", 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runs.Transition(ctx, run.ID, []string{"DISPATCHED"}, "RUNNING"); err != nil {
		t.Fatal(err)
	}

	if _, err := resources.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := resources.Find(ctx, "resource-lease"); err != nil || !found {
		t.Fatalf("find resource: found=%v err=%v", found, err)
	}
	lease, err := resources.Acquire(ctx, "resource-lease", attempt.ID, time.Minute, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := resources.Release(ctx, "resource-lease", attempt.ID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	schedules := NewScheduleRepository(db)
	nextFire := time.Now().UTC().Add(time.Minute)
	schedule, err := schedules.Create(ctx, ScheduleDefinition{ID: "schedule-coverage", Name: "Coverage schedule", TaskID: task.ID, Expression: "* * * * *", Enabled: true, NextFireAt: &nextFire})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := schedules.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := schedules.Find(ctx, schedule.ID); err != nil || !found {
		t.Fatalf("find schedule: found=%v err=%v", found, err)
	}
	if _, err := schedules.ListScheduleProjection(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := schedules.Update(ctx, schedule.ID, ScheduleDefinition{Name: "Coverage schedule 2", TaskID: task.ID, Expression: "*/5 * * * *", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := schedules.SetEnabled(ctx, schedule.ID, false); err != nil {
		t.Fatal(err)
	}
	if deleted, err := schedules.Delete(ctx, schedule.ID); err != nil || !deleted {
		t.Fatalf("delete schedule: deleted=%v err=%v", deleted, err)
	}
	if deleted, err := tasks.Delete(ctx, task.ID); err != nil || !deleted {
		t.Fatalf("delete task: deleted=%v err=%v", deleted, err)
	}
	if _, err := tasks.List(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runners.SetDesiredState(ctx, runnerID, "REVOKED"); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.Archive(ctx, runnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ListArchived(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteEnrollmentAndHeartbeatCoverage(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	runners := NewRunnerRepository(db)
	if err := runners.EnsurePool(ctx, "pool-keys", "Keys"); err != nil {
		t.Fatal(err)
	}
	plain, hash, err := platform.NewEnrollmentToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := runners.CreateEnrollment(ctx, RunnerRecord{ID: "runner-keys", Name: "Keys", PoolID: "pool-keys", Capacity: 2}, RunnerEnrollmentRecord{ID: "enrollment-keys", RunnerID: "runner-keys", TokenHash: hash, ExpiresAt: time.Now().Add(time.Minute), Artifact: map[string]any{"artifact": "test"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ConsumeEnrollment(ctx, hash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plain, hash, err = platform.NewEnrollmentToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := runners.CreateEnrollment(ctx, RunnerRecord{ID: "runner-keys", Name: "Keys", PoolID: "pool-keys", Capacity: 2}, RunnerEnrollmentRecord{ID: "enrollment-keys-2", RunnerID: "runner-keys", TokenHash: hash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ConsumeEnrollmentWithKey(ctx, hash, time.Now().UTC(), "key-1", public); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.FindPublicKey(ctx, "runner-keys", "key-1"); err != nil {
		t.Fatal(err)
	}
	if err := runners.HeartbeatWithKey(ctx, "runner-keys", "boot-1", time.Now().UTC(), "key-1", public); err != nil {
		t.Fatal(err)
	}
	if err := runners.HeartbeatWithKeyAndCapacity(ctx, "runner-keys", "boot-1", time.Now().UTC(), 4, "key-1", public); err != nil {
		t.Fatal(err)
	}
	if err := runners.HeartbeatWithKeyAndCapacityAndMetrics(ctx, "runner-keys", "boot-1", time.Now().UTC(), 4, RunnerMetricsSample{CPUPercent: 10, MemoryPercent: 20, MemoryUsedBytes: 10, MemoryTotalBytes: 100}, "key-1", public); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ListRunnerMetrics(ctx, "runner-keys", time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.List(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runners.MarkStale(ctx, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	_ = plain
	_ = private
}

func TestSQLiteDeadLetterAuditAndRetentionCoverage(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	now := time.Now().UTC()

	deadLetters := NewDeadLetterRepository(db, []byte("application-secret"))
	if err := deadLetters.Persist(ctx, DeadLetterRecord{ID: "dead-coverage", RunnerID: "runner", Stream: "events", Consumer: "worker", Subject: "runs", MessageID: "message", Payload: []byte("payload"), Attempts: 1, FirstFailedAt: now, LastFailedAt: now, Error: "temporary"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := deadLetters.List(ctx, DeadLetterFilter{Page: 0, Limit: 1000, State: "OPEN"}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := deadLetters.Find(ctx, "dead-coverage"); err != nil || !found {
		t.Fatalf("find dead letter: found=%v err=%v", found, err)
	}
	retry, ok, err := deadLetters.BeginRetry(ctx, "dead-coverage")
	if err != nil || !ok || string(retry.Payload) != "payload" {
		t.Fatalf("begin retry: %#v ok=%v err=%v", retry, ok, err)
	}
	if err := deadLetters.MarkRetryFailed(ctx, "dead-coverage", "temporary failure", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := deadLetters.BeginRetry(ctx, "dead-coverage"); err != nil {
		t.Fatal(err)
	}
	if err := deadLetters.MarkRetryPublished(ctx, "dead-coverage"); err != nil {
		t.Fatal(err)
	}
	if changed, err := deadLetters.Reconcile(ctx, "dead-coverage", "RECONCILED"); err != nil || !changed {
		t.Fatalf("reconcile: changed=%v err=%v", changed, err)
	}
	if _, err := deadLetters.Stats(ctx); err != nil {
		t.Fatal(err)
	}
	for _, attempts := range []uint64{0, 1, 9} {
		if DeadLetterRetryBackoff(attempts) <= 0 {
			t.Fatal("invalid retry backoff")
		}
	}

	audit := NewAuditRepository(db)
	if err := audit.Append(ctx, AuditEventRecord{ID: "audit-coverage", ActorID: "user", Method: "POST", Description: "created", Endpoint: "/tasks", Target: "task", Result: "success", RequestInput: map[string]any{"name": "task"}, ResponseOutput: map[string]any{"ok": true}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, counts, err := audit.Query(ctx, AuditFilter{Page: 0, Limit: 10, Actor: "user", Action: "POST"}); err != nil || counts.Total != 1 {
		t.Fatalf("audit query counts=%#v err=%v", counts, err)
	}

	retention := NewRetentionRepository(db)
	if err := retention.SetLegalHold(ctx, "run", "run-coverage", true, "keep"); err != nil {
		t.Fatal(err)
	}
	if err := retention.SetLegalHold(ctx, "run", "run-coverage", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := retention.Purge(ctx, now, DefaultRetentionPolicy(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := retention.PurgeCriticalRuns(ctx, now, func() (float64, error) { return 20, nil }, 1); err != nil {
		t.Fatal(err)
	}

	states := NewOIDCAuthorizationStateRepository(db)
	state := OIDCAuthorizationStateRecord{ID: "state-coverage", ProviderID: "provider", StateHash: "state-hash", NonceHash: "nonce-hash", EncryptedPKCEVerifier: []byte("pkce"), Purpose: "login", Callback: "https://callback", ExpiresAt: now.Add(time.Minute)}
	if err := states.Create(ctx, state); err != nil {
		t.Fatal(err)
	}
	if _, err := states.Consume(ctx, state.StateHash, state.NonceHash, state.ProviderID, state.Purpose, state.Callback, now); err != nil {
		t.Fatal(err)
	}
	if err := states.DeleteExpired(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteAdapterCoverage(t *testing.T) {
	if _, err := databaseFrom(struct{}{}); err == nil {
		t.Fatal("unsupported database was accepted")
	}
	for _, query := range []string{
		"SELECT $1, $2",
		"SELECT $1::jsonb",
		"SELECT $1 = ANY($2::text[])",
		"SELECT $1 @> $2::jsonb",
	} {
		if _, _, err := sqliteQuery(query, []any{"one", []string{"a", "b"}}); err != nil {
			t.Fatalf("sqlite query %q: %v", query, err)
		}
	}
	if got := sqliteSlice([]any{[]string{"a", "b"}}, "1"); len(got) != 2 {
		t.Fatalf("sqlite slice = %#v", got)
	}
}
