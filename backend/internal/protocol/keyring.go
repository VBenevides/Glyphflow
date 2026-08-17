package protocol

import (
	"crypto/ed25519"
	"errors"
	"time"
)

type VerificationKey struct {
	ID        string
	PublicKey ed25519.PublicKey
	NotBefore time.Time
	NotAfter  time.Time
	Revoked   bool
}

type Keyring map[string]VerificationKey

func (k Keyring) Revoke(id string) error {
	key, ok := k[id]
	if !ok {
		return errors.New("unknown signing key")
	}
	key.Revoked = true
	k[id] = key
	return nil
}

func (k Keyring) VerifyAt(envelope Envelope, domain string, now time.Time) error {
	key, ok := k[envelope.KeyID]
	if !ok {
		return errors.New("unknown signing key")
	}
	if key.ID != envelope.KeyID || key.Revoked {
		return errors.New("signing key is revoked")
	}
	if len(key.PublicKey) != ed25519.PublicKeySize {
		return errors.New("invalid signing key")
	}
	if !key.NotBefore.IsZero() && now.Before(key.NotBefore) {
		return errors.New("signing key is not yet valid")
	}
	if !key.NotAfter.IsZero() && now.After(key.NotAfter) {
		return errors.New("signing key has expired")
	}
	return envelope.Verify(key.PublicKey, domain)
}
