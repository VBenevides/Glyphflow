package protocol

import (
	"crypto/ed25519"
	"testing"
)

func TestSignatureAlgorithmIsFixed(t *testing.T) {
	if SignatureAlgorithm != "Ed25519" {
		t.Fatalf("unexpected signature algorithm: %s", SignatureAlgorithm)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("control-plane-1", []byte("payload"))
	if err := envelope.SignOrder(privateKey); err != nil {
		t.Fatal(err)
	}
	if err := envelope.VerifyOrder(publicKey); err != nil {
		t.Fatal(err)
	}
}
