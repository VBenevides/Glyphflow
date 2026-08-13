package protocol

import (
	"testing"
	"time"
)

func TestGeneratedSigningKeysSupportRotationOverlap(t *testing.T) {
	now := time.Now().UTC()
	oldKey, err := GenerateSigningKey("control-old", now.Add(-time.Minute), 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := GenerateSigningKey("control-new", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	oldEnvelope, err := oldKey.SignOrder([]byte(`{"order":"old"}`))
	if err != nil {
		t.Fatal(err)
	}
	newEnvelope, err := newKey.SignOrder([]byte(`{"order":"new"}`))
	if err != nil {
		t.Fatal(err)
	}
	keyring := Keyring{oldKey.ID: oldKey.Public, newKey.ID: newKey.Public}
	if err := keyring.VerifyAt(oldEnvelope, OrderSignatureDomain, now); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyAt(newEnvelope, OrderSignatureDomain, now); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Revoke(oldKey.ID); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyAt(oldEnvelope, OrderSignatureDomain, now); err == nil {
		t.Fatal("revoked rotation key accepted")
	}
}
