package worker

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

type startClaimRequester struct {
	response queue.Message
	request  queue.Message
}

func (r *startClaimRequester) Request(_ context.Context, request queue.Message, _ time.Duration) (queue.Message, error) {
	r.request = request
	return r.response, nil
}

func TestNATSStartClaimerVerifiesMatchingGrant(t *testing.T) {
	now := time.Now().UTC()
	controlKey, err := protocol.GenerateSigningKey("control-plane", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:runner-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claim := protocol.StartClaimPayload{Version: protocol.ProtocolVersion, RequestID: "attempt-1", RunID: "run-1", RunnerID: "runner-1", RunnerSessionID: "session-1", LeaseToken: "lease-1", Attempt: 1, FencingToken: 2, ExecutionSpecDigest: "digest", IssuedAt: now}
	grantBytes, err := json.Marshal(protocol.StartGrantPayload{StartClaimPayload: claim, GrantedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	grant := protocol.NewEnvelope(controlKey.ID, grantBytes)
	if err := grant.Sign(controlKey.Private, protocol.StartClaimSignatureDomain); err != nil {
		t.Fatal(err)
	}
	replyBytes, err := protocol.EncodeStartClaimReply(protocol.StartClaimReply{Granted: true, Grant: &grant})
	if err != nil {
		t.Fatal(err)
	}
	requester := &startClaimRequester{response: queue.Message{Data: replyBytes}}
	claimer := NewNATSStartClaimer(requester, workerKey, ed25519.PublicKey(controlKey.Public.PublicKey))
	if err := claimer.ClaimStart(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if requester.request.Subject != queue.StartClaimSubject(claim.RunnerID) {
		t.Fatalf("request subject = %q", requester.request.Subject)
	}
}

func TestNATSStartClaimerRejectsStaleClaim(t *testing.T) {
	reply, err := protocol.EncodeStartClaimReply(protocol.StartClaimReply{Error: "start claim rejected"})
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	requester := &startClaimRequester{response: queue.Message{Data: reply}}
	claimer := NewNATSStartClaimer(requester, workerKey, controlKey.Public.PublicKey)
	claim := protocol.StartClaimPayload{Version: protocol.ProtocolVersion, RequestID: "attempt-1", RunID: "run-1", RunnerID: "runner-1", RunnerSessionID: "session-1", LeaseToken: "lease-1", Attempt: 1, FencingToken: 2, ExecutionSpecDigest: "digest", IssuedAt: time.Now().UTC()}
	if err := claimer.ClaimStart(context.Background(), claim); err != ErrStartRejected {
		t.Fatalf("error = %v, want ErrStartRejected", err)
	}
}
