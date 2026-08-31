package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEncryptedSecretRepositoryReportsTaskUsageAndProtectsDeletion(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	secretID, taskID, poolID := "secret-usage-"+suffix, "task-usage-"+suffix, "pool-usage-"+suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(ctx, `DELETE FROM encrypted_secrets WHERE id = $1`, secretID)
		_, _ = pool.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	secrets := NewEncryptedSecretRepository(pool)
	if err := secrets.Upsert(ctx, EncryptedSecretRecord{ID: secretID, Name: "Task secret " + suffix, EncryptedValue: []byte("ciphertext")}); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(pool)
	if _, err := tasks.Create(ctx, TaskDefinition{ID: taskID, Name: "Secret task " + suffix, RunnerPoolID: poolID, Command: []string{"echo"}, SecretReferences: map[string]any{"TOKEN": secretID}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	statuses, err := secrets.ListStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var status EncryptedSecretStatusRecord
	for _, candidate := range statuses {
		if candidate.ID == secretID {
			status = candidate
			break
		}
	}
	if len(status.Tasks) != 1 || status.Tasks[0].ID != taskID || status.CanDelete {
		t.Fatalf("secret status = %#v", status)
	}
	if err := secrets.Delete(ctx, secretID); !errors.Is(err, ErrEncryptedSecretInUse) {
		t.Fatalf("used secret deletion error = %v", err)
	}
	if _, err := tasks.CreateVersion(ctx, taskID, TaskDefinition{Command: []string{"echo"}, SecretReferences: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	statuses, err = secrets.ListStatuses(ctx)
	status = EncryptedSecretStatusRecord{}
	for _, candidate := range statuses {
		if candidate.ID == secretID {
			status = candidate
			break
		}
	}
	if err != nil || len(status.Tasks) != 0 || !status.CanDelete {
		t.Fatalf("unused secret status = %#v, err = %v", status, err)
	}
	if err := secrets.Delete(ctx, secretID); err != nil {
		t.Fatal(err)
	}
}
