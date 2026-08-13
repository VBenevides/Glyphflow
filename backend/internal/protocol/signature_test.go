package protocol

import (
	"crypto/ed25519"
	"testing"
)

func TestSignatureCoversExactPayloadBytes(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("control-plane-1", []byte(`{"run_id":"run-1"}`))
	if err := envelope.Sign(privateKey, "test"); err != nil {
		t.Fatal(err)
	}
	if err := envelope.Verify(publicKey, "test"); err != nil {
		t.Fatal(err)
	}
	envelope.Payload = encodeBase64([]byte(`{"run_id":"run-2"}`))
	if err := envelope.Verify(publicKey, "test"); err == nil {
		t.Fatal("modified payload was accepted")
	}
}
