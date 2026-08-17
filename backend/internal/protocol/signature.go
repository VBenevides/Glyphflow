package protocol

import (
	"crypto/ed25519"
	"errors"
)

const (
	SignatureAlgorithm   = "Ed25519"
	OrderSignatureDomain = "glyphflow/order/v1"
	EventSignatureDomain = "glyphflow/event/v1"
)

func (e *Envelope) SignOrder(privateKey ed25519.PrivateKey) error {
	return e.Sign(privateKey, OrderSignatureDomain)
}

func (e *Envelope) SignEvent(privateKey ed25519.PrivateKey) error {
	return e.Sign(privateKey, EventSignatureDomain)
}

func (e Envelope) VerifyOrder(publicKey ed25519.PublicKey) error {
	return e.Verify(publicKey, OrderSignatureDomain)
}

func (e Envelope) VerifyEvent(publicKey ed25519.PublicKey) error {
	return e.Verify(publicKey, EventSignatureDomain)
}

// VerifyRawEnvelope verifies the small envelope before any caller parses the payload.
func VerifyRawEnvelope(raw []byte, publicKey ed25519.PublicKey, domain string) (Envelope, []byte, error) {
	envelope, err := DecodeEnvelope(raw)
	if err != nil {
		return Envelope{}, nil, err
	}
	if err := envelope.Verify(publicKey, domain); err != nil {
		return Envelope{}, nil, err
	}
	payload, err := envelope.PayloadBytes()
	if err != nil {
		return Envelope{}, nil, err
	}
	return envelope, payload, nil
}

func (e *Envelope) Sign(privateKey ed25519.PrivateKey, domain string) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return errors.New("invalid Ed25519 private key")
	}
	payload, err := e.PayloadBytes()
	if err != nil {
		return err
	}
	e.Signature = encodeSignature(ed25519.Sign(privateKey, signingBytes(domain, payload)))
	return nil
}

func (e Envelope) Verify(publicKey ed25519.PublicKey, domain string) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 public key")
	}
	payload, err := e.PayloadBytes()
	if err != nil {
		return err
	}
	signature, err := e.SignatureBytes()
	if err != nil {
		return err
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, signingBytes(domain, payload), signature) {
		return errors.New("invalid envelope signature")
	}
	return nil
}

func signingBytes(domain string, payload []byte) []byte {
	data := make([]byte, 0, len(domain)+1+len(payload))
	data = append(data, domain...)
	data = append(data, 0)
	return append(data, payload...)
}

func encodeSignature(signature []byte) string {
	return encodeBase64(signature)
}
