package protocol

import (
	"crypto/ed25519"
	"testing"
)

func TestSignatureDomainsAreSeparated(t *testing.T) {
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
	if err := envelope.VerifyEvent(publicKey); err == nil {
		t.Fatal("order signature verified as an event")
	}
}
