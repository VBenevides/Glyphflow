package protocol

import (
	"strings"
	"testing"
	"time"
)

func TestEventAndRunnerPayloadCodecs(t *testing.T) {
	event := EventPayload{Version: ProtocolVersion, EventID: "event-1", OrderID: "order-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease", RunnerID: "runner-1", Sequence: 1, ObservedAt: time.Now().UTC(), Type: EventCompleted, Result: "done"}
	raw, err := EncodeEventPayload(event)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEventPayload(raw)
	if err != nil || decoded.EventID != event.EventID || decoded.Result != event.Result {
		t.Fatalf("event payload = %#v, %v", decoded, err)
	}

	runner := RunnerControlPayload{Version: ProtocolVersion, Type: RunnerControlCapacity, RunnerID: "runner-1", Capacity: 4, IssuedAt: time.Now().UTC()}
	raw, err = EncodeRunnerControlPayload(runner)
	if err != nil {
		t.Fatal(err)
	}
	decodedRunner, err := DecodeRunnerControlPayload(raw)
	if err != nil || decodedRunner.RunnerID != runner.RunnerID || decodedRunner.Capacity != runner.Capacity {
		t.Fatalf("runner payload = %#v, %v", decodedRunner, err)
	}
}

func TestEventErrorLimitAndUnknownTypeValidation(t *testing.T) {
	if err := (EventPayload{Error: strings.Repeat("x", MaxEventErrorBytes)}).ValidateError(); err != nil {
		t.Fatal(err)
	}
	if err := (EventPayload{Error: strings.Repeat("x", MaxEventErrorBytes+1)}).ValidateError(); err == nil {
		t.Fatal("oversized event error accepted")
	}
	unknown := EventPayload{Version: ProtocolVersion, EventID: "event-1", OrderID: "order-1", RunnerID: "runner-1", Attempt: 1, Sequence: 1, Type: EventUnknown}
	if _, err := DecodeEventPayload(mustEncodePayload(t, unknown)); err != nil {
		t.Fatalf("valid unknown event rejected: %v", err)
	}
}

func mustEncodePayload(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := encodePayloadFrame(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
