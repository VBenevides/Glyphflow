package protocol

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVerifyOrderAndEventBeforeReplayAcceptance(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	order := OrderPayload{Version: ProtocolVersion, OrderID: "order-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease", RunnerID: "worker-1", RunnerSessionID: "session-1", IssuedAt: now.Add(-time.Second), NotBefore: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Type: OrderExecute, Command: []string{"echo"}, WorkingDir: "/tmp", DurationSeconds: 1}
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

func TestVerifyOrderRejectsInvalidStages(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	base := OrderPayload{Version: ProtocolVersion, OrderID: "order", RunID: "run", TaskID: "task", Attempt: 1, LeaseToken: "lease", RunnerID: "worker", RunnerSessionID: "session", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: OrderExecute, Command: []string{"echo"}, WorkingDir: ".", DurationSeconds: 1}
	valid := signedOrderPayload(t, private, base)
	keyring := Keyring{"key": {ID: "key", PublicKey: public}}
	for _, test := range []struct {
		name string
		raw  []byte
		keys Keyring
	}{
		{name: "malformed envelope", raw: []byte("not-json")},
		{name: "unknown key", raw: valid, keys: Keyring{}},
		{name: "malformed payload", raw: signedOrderBytes(t, private, []byte("{")), keys: keyring},
		{name: "invalid time", raw: signedOrderPayload(t, private, func() OrderPayload { item := base; item.IssuedAt = time.Time{}; return item }()), keys: keyring},
		{name: "wrong identity", raw: signedOrderPayload(t, private, func() OrderPayload { item := base; item.RunnerID = "other"; return item }()), keys: keyring},
		{name: "invalid execution", raw: signedOrderPayload(t, private, func() OrderPayload { item := base; item.Command = nil; return item }()), keys: keyring},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := VerifyOrder(test.raw, test.keys, now, "worker", "run", 1, "lease", time.Second, nil); err == nil {
				t.Fatal("invalid order was accepted")
			}
		})
	}
	guard := NewReplayGuard()
	if _, err := VerifyOrder(valid, keyring, now, "worker", "run", 1, "lease", time.Second, guard); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOrder(valid, keyring, now, "worker", "run", 1, "lease", time.Second, guard); err == nil {
		t.Fatal("replayed order was accepted")
	}
}

func TestVerifyEventRejectsInvalidStages(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	base := EventPayload{Version: ProtocolVersion, EventID: "event", OrderID: "order", RunID: "run", TaskID: "task", Attempt: 1, LeaseToken: "lease", RunnerID: "worker", RunnerSessionID: "session", Sequence: 1, ObservedAt: now, Type: EventCompleted}
	valid := signedEventPayload(t, private, base)
	keyring := Keyring{"key": {ID: "key", PublicKey: public}}
	for _, test := range []struct {
		name string
		raw  []byte
		keys Keyring
	}{
		{name: "malformed envelope", raw: []byte("not-json")},
		{name: "unknown key", raw: valid, keys: Keyring{}},
		{name: "malformed payload", raw: signedEventBytes(t, private, []byte("{")), keys: keyring},
		{name: "invalid time", raw: signedEventPayload(t, private, func() EventPayload { item := base; item.ObservedAt = time.Time{}; return item }()), keys: keyring},
		{name: "wrong identity", raw: signedEventPayload(t, private, func() EventPayload { item := base; item.RunnerID = "other"; return item }()), keys: keyring},
		{name: "wrong sequence", raw: valid, keys: keyring},
		{name: "oversized error", raw: signedEventPayload(t, private, func() EventPayload { item := base; item.Error = strings.Repeat("x", MaxEventErrorBytes+1); return item }()), keys: keyring},
	} {
		t.Run(test.name, func(t *testing.T) {
			expectedSequence := uint64(1)
			if test.name == "wrong sequence" {
				expectedSequence = 2
			}
			if _, err := VerifyEvent(test.raw, test.keys, now, "worker", "run", 1, "lease", expectedSequence, time.Second, nil); err == nil {
				t.Fatal("invalid event was accepted")
			}
		})
	}
	guard := NewReplayGuard()
	if _, err := VerifyEvent(valid, keyring, now, "worker", "run", 1, "lease", 1, time.Second, guard); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyEvent(valid, keyring, now, "worker", "run", 1, "lease", 1, time.Second, guard); err == nil {
		t.Fatal("replayed event was accepted")
	}
}

func signedOrderPayload(t *testing.T, private ed25519.PrivateKey, payload OrderPayload) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return signedOrderBytes(t, private, raw)
}

func signedOrderBytes(t *testing.T, private ed25519.PrivateKey, payload []byte) []byte {
	t.Helper()
	envelope := NewEnvelope("key", payload)
	if err := envelope.SignOrder(private); err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func signedEventPayload(t *testing.T, private ed25519.PrivateKey, payload EventPayload) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return signedEventBytes(t, private, raw)
}

func signedEventBytes(t *testing.T, private ed25519.PrivateKey, payload []byte) []byte {
	t.Helper()
	envelope := NewEnvelope("key", payload)
	if err := envelope.SignEvent(private); err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
