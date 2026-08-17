package worker

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

var (
	ErrStartRejected    = errors.New("control plane rejected start claim")
	ErrStartUnavailable = errors.New("control plane start claim unavailable")
)

type StartClaimer interface {
	ClaimStart(context.Context, protocol.StartClaimPayload) error
}

type NATSStartClaimer struct {
	requester        queue.Requester
	signingKey       protocol.SigningKey
	controlPublicKey ed25519.PublicKey
}

func NewNATSStartClaimer(requester queue.Requester, signingKey protocol.SigningKey, controlPublicKey ed25519.PublicKey) *NATSStartClaimer {
	return &NATSStartClaimer{requester: requester, signingKey: signingKey, controlPublicKey: controlPublicKey}
}

func (c *NATSStartClaimer) ClaimStart(ctx context.Context, claim protocol.StartClaimPayload) error {
	if c == nil || c.requester == nil || len(c.signingKey.Private) != ed25519.PrivateKeySize || len(c.controlPublicKey) != ed25519.PublicKeySize {
		return ErrStartUnavailable
	}
	if err := claim.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrStartUnavailable, err)
	}
	payload, err := json.Marshal(claim)
	if err != nil {
		return fmt.Errorf("%w: encode claim: %v", ErrStartUnavailable, err)
	}
	envelope := protocol.NewEnvelope(c.signingKey.ID, payload)
	if err := envelope.Sign(c.signingKey.Private, protocol.StartClaimSignatureDomain); err != nil {
		return fmt.Errorf("%w: sign claim: %v", ErrStartUnavailable, err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		return fmt.Errorf("%w: encode envelope: %v", ErrStartUnavailable, err)
	}
	response, err := c.requester.Request(ctx, queue.Message{Subject: queue.StartClaimSubject(claim.RunnerID), Data: raw}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("%w: request: %v", ErrStartUnavailable, err)
	}
	reply, err := protocol.DecodeStartClaimReply(response.Data)
	if err != nil {
		return fmt.Errorf("%w: decode reply: %v", ErrStartUnavailable, err)
	}
	if !reply.Granted {
		if reply.Retry {
			return fmt.Errorf("%w: %s", ErrStartUnavailable, reply.Error)
		}
		return ErrStartRejected
	}
	if reply.Grant == nil {
		return fmt.Errorf("%w: grant is missing", ErrStartUnavailable)
	}
	grantRaw, err := json.Marshal(reply.Grant)
	if err != nil {
		return fmt.Errorf("%w: encode grant: %v", ErrStartUnavailable, err)
	}
	_, grantBytes, err := protocol.VerifyRawEnvelope(grantRaw, c.controlPublicKey, protocol.StartClaimSignatureDomain)
	if err != nil {
		return fmt.Errorf("%w: verify grant: %v", ErrStartUnavailable, err)
	}
	var grant protocol.StartGrantPayload
	if err := json.Unmarshal(grantBytes, &grant); err != nil {
		return fmt.Errorf("%w: decode grant: %v", ErrStartUnavailable, err)
	}
	if !sameStartClaim(grant.StartClaimPayload, claim) || grant.GrantedAt.IsZero() || grant.GrantedAt.After(time.Now().UTC().Add(5*time.Second)) {
		return fmt.Errorf("%w: grant does not match claim", ErrStartUnavailable)
	}
	return nil
}

func sameStartClaim(left, right protocol.StartClaimPayload) bool {
	return left.Version == right.Version && left.RequestID == right.RequestID && left.RunID == right.RunID && left.RunnerID == right.RunnerID && left.RunnerSessionID == right.RunnerSessionID && left.LeaseToken == right.LeaseToken && left.Attempt == right.Attempt && left.FencingToken == right.FencingToken && left.ExecutionSpecDigest == right.ExecutionSpecDigest && left.IssuedAt.Equal(right.IssuedAt)
}
