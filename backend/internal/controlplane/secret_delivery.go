package controlplane

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const secretDeliveryClockTolerance = 5 * time.Second

func RunSecretDeliveryServer(ctx context.Context, requests queue.RequestServer, runs store.SecretRequestRepository, secrets store.EncryptedSecretRepository, keys RunnerKeyRepository, signingKey protocol.SigningKey, encryptionKey []byte) error {
	if requests == nil || runs == nil || secrets == nil || keys == nil || len(signingKey.Private) != ed25519.PrivateKeySize || len(encryptionKey) != 32 {
		return errors.New("secret delivery server is not configured")
	}
	return requests.ServeRequests(ctx, queue.SecretDeliverySubject(">"), func(handlerCtx context.Context, message queue.Message) queue.Message {
		return queue.Message{Data: secretDeliveryResponse(handlerCtx, runs, secrets, keys, signingKey, encryptionKey, message.Data)}
	})
}

func secretDeliveryResponse(ctx context.Context, runs store.SecretRequestRepository, secrets store.EncryptedSecretRepository, keys RunnerKeyRepository, signingKey protocol.SigningKey, encryptionKey []byte, raw []byte) []byte {
	reply := protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion, Error: "secret delivery rejected", RespondedAt: time.Now().UTC()}
	envelope, err := protocol.DecodeEnvelope(raw)
	if err != nil {
		return signSecretDeliveryResponse(reply, signingKey)
	}
	payload, err := envelope.PayloadBytes()
	if err != nil {
		return signSecretDeliveryResponse(reply, signingKey)
	}
	request, err := protocol.DecodeSecretDeliveryRequest(payload)
	if err != nil {
		return signSecretDeliveryResponse(reply, signingKey)
	}
	reply.RequestID = request.RequestID
	publicKey, err := keys.FindPublicKey(ctx, request.RunnerID, envelope.KeyID)
	if err != nil || envelope.Verify(publicKey, protocol.SecretDeliverySignatureDomain) != nil {
		return signSecretDeliveryResponse(reply, signingKey)
	}
	now := time.Now().UTC()
	if request.IssuedAt.After(now.Add(secretDeliveryClockTolerance)) || request.IssuedAt.Before(now.Add(-time.Minute)) {
		return signSecretDeliveryResponse(reply, signingKey)
	}
	if err := runs.AuthorizeSecretRequest(ctx, store.SecretRequestInput{OrderID: request.OrderID, RunID: request.RunID, RunnerID: request.RunnerID, RunnerSessionID: request.RunnerSessionID, LeaseToken: request.LeaseToken, Attempt: int(request.Attempt), FencingToken: int64(request.FencingToken), ExecutionSpecDigest: request.ExecutionSpecDigest, SecretRefs: request.SecretRefs}); err != nil {
		return signSecretDeliveryResponse(reply, signingKey)
	}
	values := make(map[string]string, len(request.SecretRefs))
	for name, id := range request.SecretRefs {
		record, found, err := secrets.Find(ctx, id)
		if err != nil || !found {
			clear(values)
			return signSecretDeliveryResponse(reply, signingKey)
		}
		if len(encryptionKey) != 32 {
			_ = secrets.SetIntegrityStatus(ctx, record.ID, store.SecretIntegrityKeyUnavailable, now)
			clear(values)
			return signSecretDeliveryResponse(reply, signingKey)
		}
		value, err := platform.DecryptSecret(encryptionKey, record.EncryptedValue)
		if err != nil {
			status := store.SecretIntegrityFailed
			if errors.Is(err, platform.ErrSecretDecryption) {
				status = store.SecretIntegrityDecryptionFailed
			}
			_ = secrets.SetIntegrityStatus(ctx, record.ID, status, now)
			clear(values)
			return signSecretDeliveryResponse(reply, signingKey)
		}
		if err := secrets.SetIntegrityStatus(ctx, record.ID, store.SecretIntegrityValid, now); err != nil {
			clear(values)
			return signSecretDeliveryResponse(reply, signingKey)
		}
		values[name] = value
	}
	reply.Error = ""
	reply.Values = values
	reply.RespondedAt = now
	return signSecretDeliveryResponse(reply, signingKey)
}

func signSecretDeliveryResponse(reply protocol.SecretDeliveryResponse, signingKey protocol.SigningKey) []byte {
	payload, err := protocol.EncodeSecretDeliveryResponse(reply)
	if reply.Values != nil {
		clear(reply.Values)
	}
	if err != nil {
		return nil
	}
	envelope := protocol.NewEnvelope(signingKey.ID, payload)
	if err := envelope.Sign(signingKey.Private, protocol.SecretDeliverySignatureDomain); err != nil {
		return nil
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		return nil
	}
	return raw
}
