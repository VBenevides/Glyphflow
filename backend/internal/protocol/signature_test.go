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

func TestAlteredPayloadIsRejected(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("control-plane-1", []byte("original"))
	if err := envelope.SignOrder(privateKey); err != nil {
		t.Fatal(err)
	}
	payload, err := envelope.PayloadBytes()
	if err != nil {
		t.Fatal(err)
	}
	payload[0] ^= 1
	envelope.Payload = encodeBase64(payload)
	if err := envelope.VerifyOrder(publicKey); err == nil {
		t.Fatal("altered payload was accepted")
	}
}
