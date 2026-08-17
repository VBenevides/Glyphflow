package protocol

import (
	"crypto/ed25519"
	"testing"
	"time"
)

func TestKeyringRejectsUnknownRevokedAndExpiredKeys(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("key-1", []byte("payload"))
	if err := envelope.SignOrder(privateKey); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	keyring := Keyring{"key-1": {ID: "key-1", PublicKey: publicKey, NotAfter: now.Add(time.Minute)}}
	if err := keyring.VerifyAt(envelope, OrderSignatureDomain, now); err != nil {
		t.Fatal(err)
	}
	if err := (Keyring{}).VerifyAt(envelope, OrderSignatureDomain, now); err == nil {
		t.Fatal("unknown key was accepted")
	}
	keyring["key-1"] = VerificationKey{ID: "key-1", PublicKey: publicKey, Revoked: true}
	if err := keyring.VerifyAt(envelope, OrderSignatureDomain, now); err == nil {
		t.Fatal("revoked key was accepted")
	}
	keyring["key-1"] = VerificationKey{ID: "key-1", PublicKey: publicKey, NotAfter: now.Add(-time.Second)}
	if err := keyring.VerifyAt(envelope, OrderSignatureDomain, now); err == nil {
		t.Fatal("expired key was accepted")
	}
	keyring["key-1"] = VerificationKey{ID: "key-1", PublicKey: []byte("invalid")}
	if err := keyring.VerifyAt(envelope, OrderSignatureDomain, now); err == nil {
		t.Fatal("invalid key was accepted")
	}
}

func TestWrongKeyIsRejected(t *testing.T) {
	wrongPublic, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("key-1", []byte("payload"))
	if err := envelope.SignOrder(privateKey); err != nil {
		t.Fatal(err)
	}
	if err := envelope.VerifyOrder(wrongPublic); err == nil {
		t.Fatal("wrong key was accepted")
	}
}

func TestKeyringRevocation(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := NewEnvelope("key-1", []byte("payload"))
	if err := envelope.SignOrder(private); err != nil {
		t.Fatal(err)
	}
	keys := Keyring{"key-1": {ID: "key-1", PublicKey: public}}
	if err := keys.Revoke("key-1"); err != nil || keys["key-1"].Revoked == false {
		t.Fatal("key was not revoked")
	}
	if err := keys.VerifyAt(envelope, OrderSignatureDomain, time.Now()); err == nil {
		t.Fatal("revoked key verified")
	}
}
