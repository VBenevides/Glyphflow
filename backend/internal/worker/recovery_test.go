package worker

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

func TestOrderRecoveryMarksOlderBootOrdersUnknown(t *testing.T) {
	recovery := NewOrderRecovery("boot-2")
	if err := recovery.Claim("order-1"); err != nil {
		t.Fatal(err)
	}
	recovery.orders["order-old"] = "boot-1"
	unknown := recovery.Recover("boot-1")
	if len(unknown) != 1 || unknown[0] != "order-old" {
		t.Fatalf("unexpected recovery result: %#v", unknown)
	}
}

func TestSignedDurableRecoveryProducesVerifiableUnknownEvent(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	control, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:runner", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "o-signed", RunID: "r-signed", TaskID: "t-signed", Attempt: 2, LeaseToken: "lease", RunnerID: "runner", RunnerSessionID: "session", FencingToken: 4, IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"true"}, WorkingDir: ".", DurationSeconds: 1}
	payload, err := protocol.EncodeOrderPayload(order)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := control.SignOrder(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutOrder(InboxOrder{OrderID: order.OrderID, ExecutionAttemptID: "a", RunID: order.RunID, TaskVersionID: order.TaskID, RunnerID: order.RunnerID, RunnerSessionID: order.RunnerSessionID, ExecutorBootID: "old", Envelope: string(raw), State: "RECEIVED", LeaseToken: order.LeaseToken, FencingToken: int64(order.FencingToken), LeaseNotAfter: order.ExpiresAt, AttemptNumber: int(order.Attempt)}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOrder(order.OrderID, "old", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverDurableSigned(store, "old", workerKey); err != nil {
		t.Fatal(err)
	}
	events, err := store.PendingEvents(10)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %#v, err=%v", events, err)
	}
	signed, err := protocol.DecodeEnvelope([]byte(events[0].Envelope))
	if err != nil {
		t.Fatal(err)
	}
	if err := signed.VerifyEvent(ed25519.PublicKey(workerKey.Public.PublicKey)); err != nil {
		t.Fatal(err)
	}
	eventRaw, err := signed.PayloadBytes()
	if err != nil {
		t.Fatal(err)
	}
	event, err := protocol.DecodeEventPayload(eventRaw)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != protocol.EventUnknown || event.Attempt != order.Attempt {
		t.Fatalf("recovery event = %#v", event)
	}
}

func TestDurableRecoveryMarksSQLiteClaimsUnknown(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PutOrder(InboxOrder{OrderID: "o", ExecutionAttemptID: "a", RunID: "r", TaskVersionID: "t", RunnerID: "runner", RunnerSessionID: "session", Envelope: "order", LeaseToken: "lease", LeaseNotAfter: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOrder("o", "old-boot", 1); err != nil {
		t.Fatal(err)
	}
	ids, err := RecoverDurable(store, "old-boot")
	if err != nil || len(ids) != 1 {
		t.Fatalf("durable recovery: %#v %v", ids, err)
	}
	events, err := store.PendingEvents(10)
	if err != nil || len(events) != 1 || events[0].EventType != "unknown" || events[0].State != "PENDING" {
		t.Fatalf("durable recovery event: %#v %v", events, err)
	}
}
