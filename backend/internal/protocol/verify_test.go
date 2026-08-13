package protocol

import (
	"crypto/ed25519"
	"testing"
)

func TestVerifyRawEnvelopeDoesNotParsePayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("control-plane-1", []byte("not-json"))
	if err := envelope.SignOrder(privateKey); err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := VerifyRawEnvelope(raw, publicKey, OrderSignatureDomain)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "not-json" {
		t.Fatalf("unexpected verified payload: %q", payload)
	}
}
