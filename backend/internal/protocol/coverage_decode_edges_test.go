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
	request := SecretDeliveryRequest{Version: ProtocolVersion, RequestID: "request", OrderID: "order", RunID: "run", Attempt: 1, LeaseToken: "lease", RunnerID: "runner", RunnerSessionID: "session", FencingToken: 1, ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret"}, IssuedAt: time.Now().UTC()}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	requestRaw, err := EncodeSecretDeliveryRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeSecretDeliveryRequest(requestRaw); err != nil || decoded.RequestID != request.RequestID {
		t.Fatalf("decoded secret request = %#v, %v", decoded, err)
	}
	response := SecretDeliveryResponse{Version: ProtocolVersion, RequestID: request.RequestID, Values: map[string]string{"TOKEN": "value"}, RespondedAt: time.Now().UTC()}
	responseRaw, err := EncodeSecretDeliveryResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeSecretDeliveryResponse(responseRaw); err != nil || decoded.Values["TOKEN"] != "value" {
		t.Fatalf("decoded secret response = %#v, %v", decoded, err)
	}
	if err := (SecretDeliveryResponse{Version: ProtocolVersion, RequestID: request.RequestID, Error: "rejected", RespondedAt: time.Now().UTC()}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, response := range []SecretDeliveryResponse{{}, {Version: ProtocolVersion, RequestID: "request", RespondedAt: time.Now().UTC(), Error: "error", Values: map[string]string{"TOKEN": "value"}}, {Version: ProtocolVersion, RequestID: "request", RespondedAt: time.Now().UTC()}} {
		if err := response.Validate(); err == nil {
			t.Fatalf("invalid secret response accepted: %#v", response)
		}
	}
}
