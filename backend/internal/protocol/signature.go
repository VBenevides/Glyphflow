package protocol

import (
	"crypto/ed25519"
	"errors"
)

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
