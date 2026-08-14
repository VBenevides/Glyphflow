//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDistributedFailureBoundaries(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	natsURL := os.Getenv("NATS_TLS_URL")
	if databaseURL == "" || natsURL == "" || os.Getenv("NATS_CERT_FILE") == "" || os.Getenv("NATS_KEY_FILE") == "" || os.Getenv("NATS_CA_FILE") == "" {
		t.Skip("set database and mutual-TLS NATS variables to run distributed failure tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	jetstream, err := queue.ConnectJetStreamTLS(natsURL, queue.TLSConfig{CertificateFile: os.Getenv("NATS_CERT_FILE"), KeyFile: os.Getenv("NATS_KEY_FILE"), CAFile: os.Getenv("NATS_CA_FILE")})
	if err != nil {
		t.Fatal(err)
	}
	defer jetstream.Close()

	name := fmt.Sprintf("distributed-%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `INSERT INTO task_definitions (id, name, schedule, timezone, command) VALUES ($1, $2, 'manual', 'UTC', '["echo","ok"]'::jsonb)`, name, name); err != nil {
		t.Fatal(err)
	}
	create := func(runID string, resources []store.ResourceLeaseInput) error {
		return store.CreateTaskRun(ctx, pool, store.CreateTaskRunParams{RunID: runID, TaskDefinitionID: name, OccurrenceAt: time.Now().UTC(), RunnerID: "runner-1", Attempt: 1, LeaseToken: "lease-" + runID, OrderBytes: []byte("signed-order"), OrderSubject: queue.Subject("orders", "runner-1"), Resources: resources})
	}
	if err := create(name+"-run-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := create(name+"-run-1", nil); err == nil {
		t.Fatal("duplicate logical run was accepted")
	}
	if err := create(name+"-lease-1", []store.ResourceLeaseInput{{ID: name + "-lease-1", ResourceKey: name + "-resource", LeaseToken: "lease-1", ExpiresAt: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if err := create(name+"-lease-2", []store.ResourceLeaseInput{{ID: name + "-lease-2", ResourceKey: name + "-resource", LeaseToken: "lease-2", ExpiresAt: time.Now().Add(time.Hour)}}); err == nil {
		t.Fatal("active resource lease takeover was accepted")
	}
	var rolledBack int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_runs WHERE id = $1`, name+"-lease-2").Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("failed transaction left task run: %d %v", rolledBack, err)
	}

	local, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if err := local.PutOrder(worker.InboxOrder{OrderID: name + "-order", ExecutionAttemptID: name + "-attempt", RunID: name + "-run-1", TaskVersionID: name, RunnerID: "runner-1", RunnerSessionID: "session-1", Envelope: "order", LeaseToken: "lease-1", LeaseNotAfter: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := local.ClaimOrder(name+"-order", "boot-1", 7); err != nil {
		t.Fatal(err)
	}
	if recovered, err := local.RecoverOrders("boot-2"); err != nil || len(recovered) != 1 {
		t.Fatalf("worker restart recovery: %#v %v", recovered, err)
	}

	subject := queue.Subject("orders", "runner-failure-test")
	messageID := name + "-message"
	if err := jetstream.Publish(ctx, queue.Message{Subject: subject, ID: messageID, Data: []byte("order")}); err != nil {
		t.Fatal(err)
	}
	if err := jetstream.Publish(ctx, queue.Message{Subject: subject, ID: messageID, Data: []byte("order")}); err != nil {
		t.Fatal(err)
	}
	consumer, err := jetstream.Consumer(ctx, "failure-"+name, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	var deliveries atomic.Int32
	for deliveries.Load() < 2 {
		if err := jetstream.ConsumeOne(ctx, consumer, func(context.Context, queue.Message) error {
			if deliveries.Add(1) == 1 {
				return fmt.Errorf("injected consumer failure")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := deliveries.Load(); got != 2 {
		t.Fatalf("expected one redelivery, got %d deliveries", got)
	}
}
