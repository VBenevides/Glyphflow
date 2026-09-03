package protocol

import (
	"testing"
	"time"
)

func TestPayloadDecodeAndEnvelopeEdges(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("bad"), mustEncodePayload(t, OrderPayload{Version: 2}), mustEncodePayload(t, OrderPayload{Version: ProtocolVersion, Type: "unknown"})} {
		if _, err := DecodeOrderPayload(raw); err == nil {
			t.Fatalf("invalid order payload accepted: %q", raw)
		}
	}
	for _, raw := range [][]byte{nil, []byte("bad"), mustEncodePayload(t, EventPayload{Version: 2}), mustEncodePayload(t, EventPayload{Version: ProtocolVersion, Type: EventUnknown})} {
		if _, err := DecodeEventPayload(raw); err == nil {
			t.Fatalf("invalid event payload accepted: %q", raw)
		}
	}
	for _, raw := range [][]byte{nil, []byte("bad"), mustEncodePayload(t, RunnerControlPayload{Version: 2}), mustEncodePayload(t, RunnerControlPayload{Version: ProtocolVersion, Type: RunnerControlCapacity, RunnerID: "runner"}), mustEncodePayload(t, RunnerControlPayload{Version: ProtocolVersion, Type: "other", RunnerID: "runner", Capacity: 1, IssuedAt: time.Now().UTC()})} {
		if _, err := DecodeRunnerControlPayload(raw); err == nil {
			t.Fatalf("invalid runner control accepted: %q", raw)
		}
	}
	for _, envelope := range []Envelope{{}, {Payload: "bad"}, {Signature: "bad", Payload: "eA=="}} {
		if _, err := envelope.PayloadBytes(); err == nil && envelope.Payload == "" {
			t.Fatal("empty envelope payload accepted")
		}
		if _, err := envelope.SignatureBytes(); err == nil && envelope.Signature == "" {
			t.Fatal("empty envelope signature accepted")
		}
	}
	if err := (EventPayload{RunnerID: "runner", RunID: "run", Attempt: 1, LeaseToken: "lease"}).ValidateIdentity("other", "run", 1, "lease"); err == nil {
		t.Fatal("mismatched event identity accepted")
	}
	if err := (EventPayload{RunnerID: "runner", RunID: "run", Attempt: 1, LeaseToken: "lease"}).ValidateIdentity("runner", "run", 1, "lease"); err != nil {
		t.Fatal(err)
	}
}
