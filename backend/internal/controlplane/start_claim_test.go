package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type startClaimTestRepository struct {
	granted bool
	input   store.StartClaimInput
}

func (r *startClaimTestRepository) ClaimStart(_ context.Context, input store.StartClaimInput) (time.Time, bool, error) {
	r.input = input
	return time.Now().UTC(), r.granted, nil
}

type startClaimTestKeys struct {
	runnerID string
	keyID    string
	public   ed25519.PublicKey
}

func (r startClaimTestKeys) FindPublicKey(_ context.Context, runnerID, keyID string) (ed25519.PublicKey, error) {
	if runnerID != r.runnerID || keyID != r.keyID {
		return nil, context.Canceled
	}
	return r.public, nil
}

func TestStartClaimResponseSignsGrantAfterRepositoryApproval(t *testing.T) {
	now := time.Now().UTC()
	runnerKey, err := protocol.GenerateSigningKey("runner:runner-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := protocol.GenerateSigningKey("control-plane", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claim := protocol.StartClaimPayload{Version: protocol.ProtocolVersion, RequestID: "attempt-1", RunID: "run-1", RunnerID: "runner-1", RunnerSessionID: "session-1", LeaseToken: "lease-1", Attempt: 1, FencingToken: 2, ExecutionSpecDigest: "digest", IssuedAt: now}
	claimBytes, err := json.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.NewEnvelope(runnerKey.ID, claimBytes)
	if err := request.Sign(runnerKey.Private, protocol.StartClaimSignatureDomain); err != nil {
		t.Fatal(err)
	}
	rawRequest, err := protocol.EncodeEnvelope(request)
	if err != nil {
		t.Fatal(err)
	}
	repository := &startClaimTestRepository{granted: true}
	rawReply := startClaimResponse(context.Background(), repository, startClaimTestKeys{runnerID: claim.RunnerID, keyID: runnerKey.ID, public: runnerKey.Public.PublicKey}, controlKey, rawRequest)
	reply, err := protocol.DecodeStartClaimReply(rawReply)
	if err != nil || !reply.Granted || reply.Grant == nil {
		t.Fatalf("reply = %#v, err=%v", reply, err)
	}
	grantRaw, err := json.Marshal(reply.Grant)
	if err != nil {
		t.Fatal(err)
	}
	_, grantBytes, err := protocol.VerifyRawEnvelope(grantRaw, controlKey.Public.PublicKey, protocol.StartClaimSignatureDomain)
	if err != nil {
		t.Fatal(err)
	}
	var grant protocol.StartGrantPayload
	if err := json.Unmarshal(grantBytes, &grant); err != nil {
		t.Fatal(err)
	}
	if grant.RunID != claim.RunID || grant.GrantedAt.IsZero() || repository.input.LeaseToken != claim.LeaseToken {
		t.Fatalf("grant = %#v, input = %#v", grant, repository.input)
	}
}
