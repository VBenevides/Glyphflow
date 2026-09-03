package worker

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

func TestLocalStoreDeduplicatesOrders(t *testing.T) {
	s, err := OpenStore(t.TempDir() + "/worker.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("order-1", map[string]string{"state": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("order-1", map[string]string{"state": "started"}); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	value, err := s.Get("order-1")
	if err != nil || string(value) != `{"state":"accepted"}` {
		t.Fatal("duplicate order changed durable state")
	}
}

func TestLocalStoreSurvivesReopen(t *testing.T) {
	path := t.TempDir() + "/worker.sqlite"
	first, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Put("order-1", map[string]string{"state": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Get("order-1"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStorePersistsRunnerConnection(t *testing.T) {
	path := t.TempDir() + "/runner.sqlite"
	first, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	connection := RunnerConnection{RunnerID: "runner-1", NATSURL: "nats://localhost:4222", MaxMessageBytes: 1024}
	if err := first.SaveConnection(connection); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	loaded, found, err := second.LoadConnection()
	if err != nil || !found || loaded != connection {
		t.Fatalf("connection = %#v, found=%t, err=%v", loaded, found, err)
	}
}

func TestLocalStoreClaimsRecoversOrdersAndPublishesEvents(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PutOrder(InboxOrder{OrderID: "o", ExecutionAttemptID: "a", RunID: "r", TaskVersionID: "t", RunnerID: "runner", RunnerSessionID: "session", Envelope: "order", LeaseToken: "lease", LeaseNotAfter: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOrder("o", "boot-1", 42); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkProcessStarted("o"); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEvent(OutboxEvent{EventID: "e", OrderID: "o", Channel: "state", Sequence: 1, EventType: "started", Envelope: "event"}); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingEvents(10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending events: %#v %v", pending, err)
	}
	if err := store.MarkEventPublished("e"); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.RecoverOrders("boot-1"); err != nil || len(ids) != 1 || ids[0] != "o" {
		t.Fatalf("recovery: %#v %v", ids, err)
	}
}

func TestLocalStoreCompactsOnlyExpiredPublishedEvents(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PutEvent(OutboxEvent{EventID: "published", OrderID: "order", Channel: "state", Sequence: 1, EventType: "started", Envelope: "event"}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkEventPublished("published"); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEvent(OutboxEvent{EventID: "pending", OrderID: "order", Channel: "state", Sequence: 2, EventType: "finished", Envelope: "event"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEvent(OutboxEvent{EventID: "only", OrderID: "other-order", Channel: "state", Sequence: 1, EventType: "started", Envelope: "event"}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkEventPublished("only"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE event_outbox SET published_at=? WHERE event_id='published'`, time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE event_outbox SET published_at=? WHERE event_id='only'`, time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.CompactPublishedEvents(24*time.Hour, 10); err != nil || removed != 1 {
		t.Fatalf("compaction removed=%d err=%v", removed, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM event_outbox WHERE event_id='pending'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("pending event count=%d err=%v", count, err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM event_outbox WHERE event_id='only'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("sequence high-water event count=%d err=%v", count, err)
	}
}

func TestLocalStoreValidationAndRecoveryBranches(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveSigningKey(protocol.SigningKey{}); err == nil {
		t.Fatal("incomplete signing key accepted")
	}
	if _, found, err := store.LoadSigningKey(); err != nil || found {
		t.Fatalf("missing signing key = found %v err %v", found, err)
	}
	if _, err := store.Get("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing metadata error = %v", err)
	}
	for _, order := range []InboxOrder{{}, {OrderID: "order"}} {
		if err := store.PutOrder(order); err == nil {
			t.Fatal("incomplete order accepted")
		}
	}
	if err := store.PutEvent(OutboxEvent{}); err == nil {
		t.Fatal("incomplete event accepted")
	}
	if _, err := store.PendingEvents(0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompactPublishedEvents(-time.Second, 1); err == nil {
		t.Fatal("negative compaction retention accepted")
	}
	if err := store.PutOrder(InboxOrder{OrderID: "order", ExecutionAttemptID: "attempt", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", TaskVersionID: "task", Envelope: "raw", LeaseToken: "lease", LeaseNotAfter: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOrder("missing", "boot", 1); err == nil {
		t.Fatal("missing order claimed")
	}
	if err := store.ClaimOrder("order", "boot", 1); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkProcessStarted("order"); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishOrder("order", "SUCCEEDED", ""); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.RecoverOrders("missing-boot"); err != nil || len(ids) != 0 {
		t.Fatalf("missing boot recovery = %#v err=%v", ids, err)
	}
	if _, err := store.RecoverOrdersSigned("", protocol.SigningKey{}); err == nil {
		t.Fatal("invalid signed recovery accepted")
	}
	if _, err := store.db.Exec(`INSERT INTO messages (id, value) VALUES (?, ?)`, "worker.signing_key", []byte("{")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadSigningKey(); err == nil {
		t.Fatal("invalid signing key metadata accepted")
	}
}
