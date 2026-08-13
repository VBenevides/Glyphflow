package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"time"
)

type SigningKey struct {
	ID        string
	Private   ed25519.PrivateKey
	Public    VerificationKey
	CreatedAt time.Time
}

func GenerateSigningKey(id string, now time.Time, validFor time.Duration) (SigningKey, error) {
	if id == "" || validFor <= 0 {
		return SigningKey{}, errors.New("signing key ID and validity are required")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SigningKey{}, err
	}
	return SigningKey{
		ID: id, Private: privateKey, CreatedAt: now,
		Public: VerificationKey{ID: id, PublicKey: publicKey, NotBefore: now, NotAfter: now.Add(validFor)},
	}, nil
}

func (k SigningKey) SignOrder(payload []byte) (Envelope, error) {
	envelope := NewEnvelope(k.ID, payload)
	if err := envelope.SignOrder(k.Private); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func (k SigningKey) SignEvent(payload []byte) (Envelope, error) {
	envelope := NewEnvelope(k.ID, payload)
	if err := envelope.SignEvent(k.Private); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}
