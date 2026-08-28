package protocol

import (
	"strings"
	"testing"
	"time"
)

func TestSecretDeliveryPayloadValidatesNamedReferences(t *testing.T) {
	request := SecretDeliveryRequest{Version: ProtocolVersion, RequestID: "attempt-1", OrderID: "attempt-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease", RunnerID: "runner-1", RunnerSessionID: "session-1", FencingToken: 1, ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret-1"}, IssuedAt: time.Now().UTC()}
	raw, err := EncodeSecretDeliveryRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSecretDeliveryRequest(raw)
	if err != nil || decoded.SecretRefs["TOKEN"] != "secret-1" {
		t.Fatalf("request = %#v, err = %v", decoded, err)
	}
	request.SecretRefs = map[string]string{"BAD-NAME": "secret-1"}
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid environment name was accepted: %v", err)
	}
}
