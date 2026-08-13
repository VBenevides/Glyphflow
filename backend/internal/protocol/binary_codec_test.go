package protocol

import (
	"bytes"
	"testing"
	"time"
)

func TestBinaryPayloadCodecIsDeterministicAndBounded(t *testing.T) {
	payload := OrderPayload{
		Version: ProtocolVersion, OrderID: "order-1", RunID: "run-1", TaskID: "task-1",
		Attempt: 1, LeaseToken: "lease", RunnerID: "runner", IssuedAt: time.Unix(1, 0).UTC(),
		NotBefore: time.Unix(1, 0).UTC(), ExpiresAt: time.Unix(2, 0).UTC(), Type: OrderExecute,
		Command: []string{"echo", "ok"}, WorkingDir: "/tmp", Resources: map[string]string{"b": "2", "a": "1"},
	}
	first, err := EncodeOrderPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncodeOrderPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || !bytes.Equal(first[:len(payloadMagic)], payloadMagic[:]) {
		t.Fatal("payload frame is not deterministic and versioned")
	}
	decoded, err := DecodeOrderPayload(first)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OrderID != payload.OrderID || decoded.Resources["a"] != "1" {
		t.Fatalf("decoded payload mismatch: %#v", decoded)
	}
	if _, err := DecodeOrderPayload(append(first, 0)); err == nil {
		t.Fatal("trailing frame bytes were accepted")
	}
}
