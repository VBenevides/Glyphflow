package protocol

import (
	"crypto/ed25519"
	"testing"
	"time"
)

func TestKeyRotationOverlap(t *testing.T) {
	oldPublic, oldPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	newPublic, newPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	keyring := Keyring{
		"old": {ID: "old", PublicKey: oldPublic, NotAfter: now.Add(time.Hour)},
		"new": {ID: "new", PublicKey: newPublic, NotBefore: now.Add(-time.Minute)},
	}
	oldEnvelope := NewEnvelope("old", []byte("payload"))
	newEnvelope := NewEnvelope("new", []byte("payload"))
	if err := oldEnvelope.SignOrder(oldPrivate); err != nil {
		t.Fatal(err)
	}
	if err := newEnvelope.SignOrder(newPrivate); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyAt(oldEnvelope, OrderSignatureDomain, now); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyAt(newEnvelope, OrderSignatureDomain, now); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyAt(oldEnvelope, OrderSignatureDomain, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired old key remained valid after rotation")
	}
}
