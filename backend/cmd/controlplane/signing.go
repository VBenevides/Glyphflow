package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

func loadControlPlaneSigningKey(encoded string) (protocol.SigningKey, error) {
	if encoded == "" {
		generated, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), 365*24*time.Hour)
		if err != nil {
			return protocol.SigningKey{}, err
		}
		encoded = base64.RawStdEncoding.EncodeToString(generated.Private)
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return protocol.SigningKey{}, errors.New("control-plane signing key is invalid")
	}
	privateKey := ed25519.PrivateKey(raw)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	now := time.Now().UTC()
	return protocol.SigningKey{ID: "control-plane", Private: privateKey, Public: protocol.VerificationKey{ID: "control-plane", PublicKey: publicKey, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour)}}, nil
}
