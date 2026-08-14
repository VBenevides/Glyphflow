package worker

import (
	"testing"
	"time"
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
