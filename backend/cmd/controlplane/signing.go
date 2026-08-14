package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const controlPlaneSigningKeyConfig = "CONTROL_PLANE_SIGNING_PRIVATE_KEY"

func loadControlPlaneSigningKey(ctx context.Context, config *store.ConfigStore) (protocol.SigningKey, error) {
	var encoded string
	found, err := config.Get(ctx, controlPlaneSigningKeyConfig, &encoded)
	if err != nil {
		return protocol.SigningKey{}, err
	}
	if !found || encoded == "" {
		generated, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), 365*24*time.Hour)
		if err != nil {
			return protocol.SigningKey{}, err
		}
		encoded = base64.RawStdEncoding.EncodeToString(generated.Private)
		if err := config.SetIfAbsent(ctx, controlPlaneSigningKeyConfig, encoded); err != nil {
			return protocol.SigningKey{}, err
		}
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return protocol.SigningKey{}, errors.New("stored control-plane signing key is invalid")
	}
	privateKey := ed25519.PrivateKey(raw)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	now := time.Now().UTC()
	return protocol.SigningKey{ID: "control-plane", Private: privateKey, Public: protocol.VerificationKey{ID: "control-plane", PublicKey: publicKey, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour)}}, nil
}
