package protocol

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifyOrderAndEventBeforeReplayAcceptance(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	order := OrderPayload{Version: ProtocolVersion, OrderID: "order-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease", RunnerID: "worker-1", IssuedAt: now.Add(-time.Second), NotBefore: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Type: OrderExecute}
	orderBytes, _ := json.Marshal(order)
	orderEnvelope := NewEnvelope("key", orderBytes)
	_ = orderEnvelope.SignOrder(private)
	rawOrder, _ := EncodeEnvelope(orderEnvelope)
	guard := NewReplayGuard()
	if _, err := VerifyOrder(rawOrder, Keyring{"key": {ID: "key", PublicKey: public}}, now, "worker-1", "run-1", 1, "lease", time.Second, guard); err != nil {
		t.Fatal(err)
	}
	event := EventPayload{Version: ProtocolVersion, EventID: "event-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease", RunnerID: "worker-1", Sequence: 1, ObservedAt: now, Type: EventCompleted}
	eventBytes, _ := json.Marshal(event)
	eventEnvelope := NewEnvelope("key", eventBytes)
	_ = eventEnvelope.SignEvent(private)
	rawEvent, _ := EncodeEnvelope(eventEnvelope)
	if _, err := VerifyEvent(rawEvent, Keyring{"key": {ID: "key", PublicKey: public}}, now, "worker-1", "run-1", 1, "lease", 1, time.Second, guard); err != nil {
		t.Fatal(err)
	}
}

func TestReplayLogSurvivesRestart(t *testing.T) {
	path := t.TempDir() + "/replay.log"
	first, err := OpenReplayLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Accept("event-1"); err != nil {
		t.Fatal(err)
	}
	second, err := OpenReplayLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Accept("event-1"); err == nil {
		t.Fatal("replay survived restart")
	}
}
