package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const startClaimClockTolerance = 5 * time.Second

func RunStartClaimServer(ctx context.Context, events queue.RequestServer, runs store.StartClaimRepository, keys RunnerKeyRepository, signingKey protocol.SigningKey) error {
	if events == nil || runs == nil || keys == nil || len(signingKey.Private) != ed25519.PrivateKeySize {
		return errors.New("start claim server is not configured")
	}
	return events.ServeRequests(ctx, queue.StartClaimSubject(">"), func(handlerCtx context.Context, message queue.Message) queue.Message {
		return queue.Message{Data: startClaimResponse(handlerCtx, runs, keys, signingKey, message.Data)}
	})
}

func startClaimResponse(ctx context.Context, runs store.StartClaimRepository, keys RunnerKeyRepository, signingKey protocol.SigningKey, raw []byte) []byte {
	reply := protocol.StartClaimReply{Error: "start claim rejected"}
	envelope, err := protocol.DecodeEnvelope(raw)
	if err != nil {
		return encodeStartClaimReply(reply)
	}
	payloadBytes, err := envelope.PayloadBytes()
	if err != nil {
		return encodeStartClaimReply(reply)
	}
	var claim protocol.StartClaimPayload
	if json.Unmarshal(payloadBytes, &claim) != nil || claim.Validate() != nil {
		return encodeStartClaimReply(reply)
	}
	publicKey, err := keys.FindPublicKey(ctx, claim.RunnerID, envelope.KeyID)
	if err != nil || envelope.Verify(publicKey, protocol.StartClaimSignatureDomain) != nil {
		return encodeStartClaimReply(reply)
	}
	now := time.Now().UTC()
	if claim.IssuedAt.After(now.Add(startClaimClockTolerance)) || claim.IssuedAt.Before(now.Add(-time.Minute)) {
		return encodeStartClaimReply(reply)
	}
	grantedAt, granted, err := runs.ClaimStart(ctx, store.StartClaimInput{RunID: claim.RunID, RunnerID: claim.RunnerID, RunnerSessionID: claim.RunnerSessionID, LeaseToken: claim.LeaseToken, Attempt: int(claim.Attempt), FencingToken: int64(claim.FencingToken), ExecutionSpecDigest: claim.ExecutionSpecDigest})
	if err != nil {
		reply.Retry = true
		reply.Error = "start claim unavailable"
		return encodeStartClaimReply(reply)
	}
	if !granted {
		return encodeStartClaimReply(reply)
	}
	grantPayload, err := json.Marshal(protocol.StartGrantPayload{StartClaimPayload: claim, GrantedAt: grantedAt})
	if err != nil {
		reply.Retry = true
		reply.Error = "start grant encoding failed"
		return encodeStartClaimReply(reply)
	}
	grant := protocol.NewEnvelope(signingKey.ID, grantPayload)
	if err := grant.Sign(signingKey.Private, protocol.StartClaimSignatureDomain); err != nil {
		reply.Retry = true
		reply.Error = "start grant signing failed"
		return encodeStartClaimReply(reply)
	}
	reply.Granted = true
	reply.Error = ""
	reply.Grant = &grant
	return encodeStartClaimReply(reply)
}

func encodeStartClaimReply(reply protocol.StartClaimReply) []byte {
	raw, _ := protocol.EncodeStartClaimReply(reply)
	return raw
}
