//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
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

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolID, runnerID := "integration-pool-"+suffix, "integration-runner-"+suffix
	taskID, runID := "integration-task-"+suffix, "integration-run-"+suffix
	resourceID, resourceRunID := "integration-resource-"+suffix, "integration-resource-run-"+suffix
	resourceAttemptID := "integration-resource-attempt-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM runs WHERE id IN ($1, $2)`, runID, resourceRunID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM resources WHERE id = $1`, resourceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM runners WHERE id = $1`, runnerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})

	runners := store.NewRunnerRepository(pool)
	if err := runners.EnsurePool(ctx, poolID, poolID); err != nil {
		t.Fatal(err)
	}
	runnerKey, err := protocol.GenerateSigningKey("runner-key-"+suffix, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := runners.CreateEnrollment(ctx, store.RunnerRecord{ID: runnerID, Name: runnerID, PoolID: poolID, Capacity: 1}, store.RunnerEnrollmentRecord{ID: "enrollment-" + suffix, RunnerID: runnerID, TokenHash: platform.HashToken("token-" + suffix), ExpiresAt: time.Now().Add(time.Minute), Target: runnerID}); err != nil {
		t.Fatal(err)
	}
	if _, err := runners.ConsumeEnrollmentWithKey(ctx, platform.HashToken("token-"+suffix), time.Now().UTC(), runnerKey.ID, runnerKey.Public.PublicKey); err != nil {
		t.Fatal(err)
	}
	bootID := "boot-" + suffix
	if err := runners.HeartbeatWithKey(ctx, runnerID, bootID, time.Now().UTC(), runnerKey.ID, runnerKey.Public.PublicKey); err != nil {
		t.Fatal(err)
	}

	tasks := store.NewTaskRepository(pool)
	if _, err := tasks.Create(ctx, store.TaskDefinition{ID: taskID, Name: taskID, RunnerPoolID: poolID, Command: []string{"echo", "ok"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	runs := store.NewRunRepository(pool)
	if _, err := runs.Create(ctx, store.RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "idempotency-" + suffix, ScheduledFor: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Create(ctx, store.RunDefinition{ID: runID + "-duplicate", TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "idempotency-" + suffix}); err == nil {
		t.Fatal("duplicate logical run was accepted")
	}

	controlKey, err := protocol.GenerateSigningKey("control-plane-"+suffix, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	orders := queue.Subject("orders", runnerID)
	consumer, err := jetstream.Consumer(ctx, "orders-"+suffix, orders, 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatchCtx, stopDispatcher := context.WithCancel(ctx)
	defer stopDispatcher()
	dispatchErrors := make(chan error, 1)
	go func() {
		if err := controlplane.RunDispatcher(dispatchCtx, jetstream, runs, runners, controlKey, time.Millisecond); err != nil && dispatchCtx.Err() == nil {
			dispatchErrors <- err
		}
	}()
	var orderMessage queue.Message
	if err := jetstream.ConsumeOne(ctx, consumer, func(_ context.Context, message queue.Message) error {
		orderMessage = message
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-dispatchErrors:
		t.Fatal(err)
	default:
	}
	orderEnvelope, err := protocol.DecodeEnvelope(orderMessage.Data)
	if err != nil {
		t.Fatal(err)
	}
	orderPayload, err := orderEnvelope.PayloadBytes()
	if err != nil {
		t.Fatal(err)
	}
	order, err := protocol.DecodeOrderPayload(orderPayload)
	if err != nil || order.RunID != runID || order.RunnerID != runnerID || order.Type != protocol.OrderExecute {
		t.Fatalf("dispatched order = %#v, err = %v", order, err)
	}

	local, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if err := local.PutOrder(worker.InboxOrder{OrderID: order.OrderID, ExecutionAttemptID: order.OrderID, RunID: order.RunID, TaskVersionID: order.TaskID, RunnerID: order.RunnerID, RunnerSessionID: order.RunnerSessionID, Envelope: string(orderMessage.Data), LeaseToken: order.LeaseToken, FencingToken: int64(order.FencingToken), LeaseNotAfter: order.LeaseNotAfter, ExecutionSpecDigest: order.ExecutionSpecDigest, AttemptNumber: int(order.Attempt)}); err != nil {
		t.Fatal(err)
	}
	if err := local.ClaimOrder(order.OrderID, bootID, 7); err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("worker-key-"+suffix, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := local.RecoverOrdersSigned(bootID, workerKey); err != nil || len(recovered) != 1 || recovered[0] != order.OrderID {
		t.Fatalf("worker restart recovery: %#v %v", recovered, err)
	}
	events, err := local.PendingEvents(1)
	if err != nil || len(events) != 1 {
		t.Fatalf("worker recovery events: %#v %v", events, err)
	}
	signedEvent, err := protocol.DecodeEnvelope([]byte(events[0].Envelope))
	if err != nil || signedEvent.VerifyEvent(workerKey.Public.PublicKey) != nil {
		t.Fatalf("worker recovery event is not signed: %v", err)
	}

	if _, err := runs.Create(ctx, store.RunDefinition{ID: resourceRunID, TaskID: taskID, TriggerType: "MANUAL", ScheduledFor: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := runs.CreateAttempt(ctx, store.ExecutionAttemptDefinition{ID: resourceAttemptID, RunID: resourceRunID, RunnerID: runnerID, RunnerSessionID: runnerID + "/" + bootID, AttemptNumber: 1, LeaseToken: "lease-" + suffix, FencingToken: 1, LeaseNotAfter: time.Now().Add(time.Minute), ExecutionSpecDigest: "digest"}); err != nil {
		t.Fatal(err)
	}
	resources := store.NewResourceRepository(pool)
	if err := resources.Create(ctx, resourceID, resourceID, "exclusive"); err != nil {
		t.Fatal(err)
	}
	lease, err := resources.Acquire(ctx, resourceID, resourceAttemptID, time.Minute, time.Now().UTC())
	if err != nil || lease.FencingToken != 1 {
		t.Fatalf("first resource lease = %#v, err = %v", lease, err)
	}
	if _, err := resources.Acquire(ctx, resourceID, resourceAttemptID, time.Minute, time.Now().UTC()); err == nil {
		t.Fatal("active resource lease takeover was accepted")
	}
	if err := resources.Release(ctx, resourceID, resourceAttemptID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	failureSubject := queue.Subject("orders", "runner-failure-"+suffix)
	failureConsumer, err := jetstream.Consumer(ctx, "failure-"+suffix, failureSubject, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := jetstream.Publish(ctx, queue.Message{Subject: failureSubject, ID: "failure-" + suffix, Data: []byte("order")}); err != nil {
		t.Fatal(err)
	}
	redeliveryCtx, cancelRedelivery := context.WithTimeout(ctx, 5*time.Second)
	defer cancelRedelivery()
	var deliveries atomic.Int32
	for deliveries.Load() < 2 {
		if err := jetstream.ConsumeOne(redeliveryCtx, failureConsumer, func(context.Context, queue.Message) error {
			if deliveries.Add(1) == 1 {
				return fmt.Errorf("injected consumer failure")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
