package store

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestSQLiteControlPlaneRepositoriesShareOneDatabase(t *testing.T) { // NOSONAR: this integration-style SQLite scenario intentionally verifies shared control-plane repository state in one fixture.
	db, err := OpenSQLite(t.TempDir() + "/controlplane.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if max := db.Stats().MaxOpenConnections; max != 8 {
		t.Fatalf("SQLite max open connections = %d, want 8", max)
	}
	ctx := context.Background()
	first, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var firstTimeout, secondTimeout int
	if err := first.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&firstTimeout); err != nil {
		t.Fatal(err)
	}
	if err := second.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&secondTimeout); err != nil {
		t.Fatal(err)
	}
	if firstTimeout != 30000 || secondTimeout != 30000 {
		t.Fatalf("SQLite busy timeouts = %d/%d, want 30000/30000", firstTimeout, secondTimeout)
	}
	first.Close()
	second.Close()
	if err := ApplySQLiteMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if err := NewRoleRepository(db).Ensure(ctx, "role-1", "user", "", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := NewUserRepository(db).Create(ctx, UserRecord{ID: "user-1", Username: "user", Email: "user@example.com", Enabled: true}, ""); err != nil {
		t.Fatal(err)
	}
	if err := NewRoleRepository(db).Assign(ctx, "user-1", "role-1", "manual", "test"); err != nil {
		t.Fatal(err)
	}
	user, found, err := NewUserRepository(db).FindByID(ctx, "user-1")
	if err != nil || !found || user.Email != "user@example.com" {
		t.Fatalf("user = %#v, found=%v, err=%v", user, found, err)
	}
	roles, _, err := NewRoleRepository(db).UserRoles(ctx, "user-1")
	if err != nil || len(roles) != 1 || roles[0].ID != "role-1" {
		t.Fatalf("roles = %#v, err=%v", roles, err)
	}
	session := SessionRecord{ID: "session-1", UserID: "user-1", RefreshTokenHash: "hash", SessionFamilyID: "family", AccessExpiresAt: time.Now().Add(time.Hour), RefreshExpiresAt: time.Now().Add(2 * time.Hour), LastSeenAt: time.Now()}
	if err := NewSessionRepository(db).Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	sessionRepository := NewSessionRepository(db)
	if _, found, err := sessionRepository.Get(ctx, session.ID); err != nil || !found {
		t.Fatalf("session found=%v, err=%v", found, err)
	}
	if active, err := sessionRepository.Active(ctx, session.ID, session.UserID); err != nil || !active {
		t.Fatalf("session active=%v, err=%v", active, err)
	}
	if err := NewRunnerRepository(db).EnsurePool(ctx, "pool-1", "Pool"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runners (id, pool_id, name, capabilities) VALUES ('runner-1', 'pool-1', 'Runner', '{"platform":"linux"}')`); err != nil {
		t.Fatal(err)
	}
	runnerRepository := NewRunnerRepository(db)
	if runners, err := runnerRepository.List(ctx); err != nil || len(runners) != 1 || runners[0].ID != "runner-1" {
		t.Fatalf("runners = %#v, err=%v", runners, err)
	}
	task, err := NewTaskRepository(db).Create(ctx, TaskDefinition{ID: "task-1", Name: "Task", RunnerPoolID: "pool-1", Command: []string{"echo", "hello"}, PlacementSelectors: map[string]any{"platform": "linux"}, SecretReferences: map[string]any{"TOKEN": "secret-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if task.Name != "Task" || len(task.Command) != 2 {
		t.Fatalf("task = %#v", task)
	}
	if versions, err := NewTaskRepository(db).ListVersions(ctx, task.ID); err != nil || len(versions) != 1 {
		t.Fatalf("versions = %#v, err=%v", versions, err)
	}
	nextFire := time.Now().In(time.FixedZone("UTC+00:00", 0)).Add(time.Minute)
	scheduleRepository := NewScheduleRepository(db)
	schedule, err := scheduleRepository.Create(ctx, ScheduleDefinition{ID: "schedule-1", Name: "Schedule", TaskID: task.ID, Expression: "* * * * *", NextFireAt: &nextFire})
	if err != nil {
		t.Fatal(err)
	}
	if schedules, err := scheduleRepository.List(ctx); err != nil || len(schedules) != 1 || schedules[0].ID != schedule.ID {
		t.Fatalf("schedules = %#v, err=%v", schedules, err)
	}
	runRepository := NewRunRepository(db)
	run, err := runRepository.Create(ctx, RunDefinition{ID: "run-1", TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "WAITING" {
		t.Fatalf("run = %#v", run)
	}
	if err := runnerRepository.CreateSession(ctx, "runner-1", "boot-1"); err != nil {
		t.Fatal(err)
	}
	if err := runnerRepository.Heartbeat(ctx, "runner-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if candidate, found, err := runRepository.ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return []byte("order"), nil }); err != nil || !found || candidate.RunID != "run-1" {
		t.Fatalf("claim = %#v, found=%v, err=%v", candidate, found, err)
	}
	secrets := NewEncryptedSecretRepository(db)
	if err := secrets.Upsert(ctx, EncryptedSecretRecord{ID: "secret-1", Name: "Secret", EncryptedValue: []byte("value")}); err != nil {
		t.Fatal(err)
	}
	if statuses, err := secrets.ListStatuses(ctx); err != nil || len(statuses) != 1 || statuses[0].CanDelete || len(statuses[0].Tasks) != 1 {
		t.Fatalf("secret statuses = %#v, err=%v", statuses, err)
	}
}

func TestSQLiteTimeAcceptsLegacyGoString(t *testing.T) {
	got, err := sqliteTime("2026-09-02 02:00:00.468807012 +0000 UTC+00:00")
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-09-02T02:00:00.468807012Z"; got.UTC().Format(time.RFC3339Nano) != want {
		t.Fatalf("SQLite time = %s, want %s", got.UTC().Format(time.RFC3339Nano), want)
	}
}

func TestSQLiteRunnerEnrollmentCanBeConsumed(t *testing.T) {
	db, err := OpenSQLite(t.TempDir() + "/controlplane.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := ApplySQLiteMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	repository := NewRunnerRepository(db)
	if err := repository.EnsurePool(ctx, "pool-enroll", "Pool"); err != nil {
		t.Fatal(err)
	}
	token, hash, err := platform.NewEnrollmentToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateEnrollment(ctx, RunnerRecord{ID: "runner-enroll", Name: "Runner", PoolID: "pool-enroll"}, RunnerEnrollmentRecord{ID: "enrollment-enroll", RunnerID: "runner-enroll", TokenHash: hash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConsumeEnrollmentWithKey(ctx, platform.HashToken(token), time.Now(), "runner:runner-enroll", make([]byte, ed25519.PublicKeySize)); err != nil {
		t.Fatal(err)
	}
}
