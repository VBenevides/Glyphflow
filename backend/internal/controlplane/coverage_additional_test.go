package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type coverageEventStream struct{ published []queue.Message }

func (s *coverageEventStream) Publish(_ context.Context, message queue.Message) error {
	s.published = append(s.published, message)
	return nil
}

func (*coverageEventStream) ConsumeSubject(context.Context, string, string, int, queue.Handler) error {
	return nil
}

type coverageDispatchRepository struct {
	waiting, cancelling int
	pending             []store.DispatchOutboxRecord
}

func (r *coverageDispatchRepository) ClaimWaiting(_ context.Context, _ func(store.DispatchCandidate) ([]byte, error)) (store.DispatchCandidate, bool, error) {
	r.waiting++
	if r.waiting > 1 {
		return store.DispatchCandidate{}, false, nil
	}
	return store.DispatchCandidate{RunID: "run", TaskID: "task", TaskVersionID: "version", AttemptID: "attempt", TaskName: "task", TaskVersion: 1, Pool: "pool", RunnerID: "runner", RunnerSessionID: "session", Command: []string{"true"}, WorkingDirectory: ".", AttemptNumber: 1, DurationSeconds: 1, MaxOutputBytes: 100, FencingToken: 1, LeaseToken: "lease", LeaseNotAfter: time.Now().Add(time.Minute), ExecutionSpecDigest: "digest"}, true, nil
}

func (r *coverageDispatchRepository) ClaimCancelling(_ context.Context, _ func(store.CancellationCandidate) ([]byte, error)) (store.CancellationCandidate, bool, error) {
	r.cancelling++
	if r.cancelling > 1 {
		return store.CancellationCandidate{}, false, nil
	}
	return store.CancellationCandidate{RunID: "run", TaskID: "task", AttemptID: "attempt", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", AttemptNumber: 1, FencingToken: 1, LeaseNotAfter: time.Now().Add(time.Minute), Reason: "operator"}, true, nil
}

func (*coverageDispatchRepository) ReconcileTimedOutDispatches(context.Context, time.Time) error {
	return nil
}
func (*coverageDispatchRepository) ReconcileStaleCancellations(context.Context, time.Time) error {
	return nil
}
func (r *coverageDispatchRepository) PendingDispatch(context.Context, int) ([]store.DispatchOutboxRecord, error) {
	pending := r.pending
	r.pending = nil
	return pending, nil
}
func (*coverageDispatchRepository) MarkDispatchPublished(context.Context, string) error { return nil }
func (*coverageDispatchRepository) RetryDispatch(context.Context, string, error) error  { return nil }
func (*coverageDispatchRepository) ApplyRunEvent(context.Context, store.RunEventInput) error {
	return nil
}
func (*coverageDispatchRepository) FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error) {
	return make(ed25519.PublicKey, ed25519.PublicKeySize), nil
}
func (r *coverageDispatchRepository) Heartbeat(context.Context, string, time.Time) error { return nil }
func (r *coverageDispatchRepository) MarkStale(context.Context, time.Time) error         { return nil }

type coverageDueRepository struct{}

func (coverageDueRepository) CreateDueRun(context.Context, time.Time, func(store.DueScheduleRecord) (time.Time, error)) (string, bool, error) {
	return "", false, nil
}

type coverageRequestServer struct{}

func (coverageRequestServer) ServeRequests(context.Context, string, queue.RequestHandler) error {
	return nil
}

func TestControlPlaneLoopConfigurationAndDispatchCoverage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := &coverageEventStream{}
	repository := &coverageDispatchRepository{pending: []store.DispatchOutboxRecord{{MessageID: "message", Subject: "subject", Envelope: []byte("envelope")}}}
	key, err := protocol.GenerateSigningKey("control", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunDispatcher(ctx, stream, repository, repository, key, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := RunScheduler(ctx, coverageDueRepository{}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := RunRunnerHeartbeatMonitor(ctx, stream, repository, time.Minute, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := dispatchWaiting(context.Background(), stream, repository, key); err != nil {
		t.Fatal(err)
	}
	if err := dispatchCancellations(context.Background(), stream, repository, key); err != nil {
		t.Fatal(err)
	}
	if err := publishPending(context.Background(), stream, repository); err != nil {
		t.Fatal(err)
	}
	if len(stream.published) != 1 || stream.published[0].ID != "message" {
		t.Fatalf("published = %#v", stream.published)
	}
	if err := RunDispatcher(context.Background(), nil, repository, repository, key, time.Millisecond); err == nil {
		t.Fatal("invalid dispatcher configuration accepted")
	}
	if err := RunScheduler(context.Background(), nil, time.Millisecond); err == nil {
		t.Fatal("invalid scheduler configuration accepted")
	}
}

func TestControlPlaneRequestServerConfigurationCoverage(t *testing.T) {
	key, err := protocol.GenerateSigningKey("control", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	repository := &secretDeliveryTestRepository{}
	keys := secretDeliveryTestKeys{}
	if err := RunSecretDeliveryServer(context.Background(), nil, repository, repository, keys, key, make([]byte, 32)); err == nil {
		t.Fatal("invalid secret delivery configuration accepted")
	}
	if err := RunSecretDeliveryServer(context.Background(), coverageRequestServer{}, repository, repository, keys, key, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := RunStartClaimServer(context.Background(), nil, &startClaimTestRepository{}, keys, key); err == nil {
		t.Fatal("invalid start claim configuration accepted")
	}
	if err := RunStartClaimServer(context.Background(), coverageRequestServer{}, &startClaimTestRepository{}, keys, key); err != nil {
		t.Fatal(err)
	}
}

type coverageStartClaimRepository struct {
	granted bool
	err     error
}

type coverageEventKeys struct {
	key protocol.SigningKey
	err error
}

func (k coverageEventKeys) FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error) {
	if k.err != nil {
		return nil, k.err
	}
	return k.key.Public.PublicKey, nil
}

type coverageEventRepository struct {
	*coverageDispatchRepository
	event store.RunEventInput
}

func (r *coverageEventRepository) ApplyRunEvent(_ context.Context, event store.RunEventInput) error {
	r.event = event
	return nil
}

func TestApplyRunnerEventBranchesCoverage(t *testing.T) {
	now := time.Now().UTC()
	key, err := protocol.GenerateSigningKey("runner:events", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	code := 0
	payload, err := protocol.EncodeEventPayload(protocol.EventPayload{Version: protocol.ProtocolVersion, EventID: "event", OrderID: "attempt", RunID: "run", TaskID: "task", Attempt: 1, LeaseToken: "lease", RunnerID: "runner", Sequence: 1, ObservedAt: now, Type: protocol.EventCompleted, ExitCode: &code, RunnerSessionID: "session", FencingToken: 1, EventChannel: "state"})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := key.SignEvent(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	repository := &coverageEventRepository{coverageDispatchRepository: &coverageDispatchRepository{}}
	keys := coverageEventKeys{key: key}
	if err := applyRunnerEvent(context.Background(), keys, repository, queue.Message{Subject: queue.Subject("events", "runner"), Data: raw}); err != nil {
		t.Fatal(err)
	}
	if repository.event.EventID != "event" || repository.event.RunnerID != "runner" {
		t.Fatalf("applied event = %#v", repository.event)
	}
	for _, message := range []queue.Message{
		{Subject: "bad", Data: raw},
		{Subject: queue.Subject("events", "runner"), Data: []byte("bad")},
		{Subject: queue.Subject("events", "runner"), Data: mustEnvelopeBytes(t, key, []byte("bad"))},
	} {
		if err := applyRunnerEvent(context.Background(), keys, repository, message); err == nil {
			t.Fatalf("invalid event accepted: %#v", message)
		}
	}
	if err := applyRunnerEvent(context.Background(), coverageEventKeys{err: errors.New("key unavailable")}, repository, queue.Message{Subject: queue.Subject("events", "runner"), Data: raw}); err == nil {
		t.Fatal("event with unavailable key accepted")
	}
	tampered := append([]byte(nil), raw...)
	tampered[len(tampered)-1] ^= 1
	if err := applyRunnerEvent(context.Background(), keys, repository, queue.Message{Subject: queue.Subject("events", "runner"), Data: tampered}); err == nil {
		t.Fatal("tampered event accepted")
	}
}

func (r coverageStartClaimRepository) ClaimStart(context.Context, store.StartClaimInput) (time.Time, bool, error) {
	return time.Now().UTC(), r.granted, r.err
}

func TestStartClaimRejectionBranchesCoverage(t *testing.T) {
	now := time.Now().UTC()
	runnerKey, err := protocol.GenerateSigningKey("runner:coverage", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claim := protocol.StartClaimPayload{Version: protocol.ProtocolVersion, RequestID: "request", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Attempt: 1, FencingToken: 1, ExecutionSpecDigest: "digest", IssuedAt: now}
	claimBytes, _ := json.Marshal(claim)
	request := protocol.NewEnvelope(runnerKey.ID, claimBytes)
	if err := request.Sign(runnerKey.Private, protocol.StartClaimSignatureDomain); err != nil {
		t.Fatal(err)
	}
	raw, _ := protocol.EncodeEnvelope(request)
	keys := startClaimTestKeys{runnerID: claim.RunnerID, keyID: runnerKey.ID, public: runnerKey.Public.PublicKey}
	for _, test := range []struct {
		name string
		raw  []byte
		keys RunnerKeyFinder
		runs store.StartClaimer
		key  protocol.SigningKey
		want string
	}{
		{name: "malformed", raw: []byte("{"), keys: keys, runs: coverageStartClaimRepository{granted: true}, key: runnerKey, want: "start claim rejected"},
		{name: "invalid payload", raw: mustEnvelopeBytes(t, runnerKey, []byte(`{"run_id":"run"}`)), keys: keys, runs: coverageStartClaimRepository{granted: true}, key: runnerKey, want: "start claim rejected"},
		{name: "unknown key", raw: raw, keys: startClaimTestKeys{runnerID: "other", keyID: runnerKey.ID, public: runnerKey.Public.PublicKey}, runs: coverageStartClaimRepository{granted: true}, key: runnerKey, want: "start claim rejected"},
		{name: "stale", raw: mustEnvelopeBytes(t, runnerKey, cloneStartClaim(claim, func(p *protocol.StartClaimPayload) { p.IssuedAt = now.Add(-2 * time.Minute) })), keys: keys, runs: coverageStartClaimRepository{granted: true}, key: runnerKey, want: "start claim rejected"},
		{name: "repository error", raw: raw, keys: keys, runs: coverageStartClaimRepository{err: errors.New("database unavailable")}, key: runnerKey, want: "start claim unavailable"},
		{name: "not granted", raw: raw, keys: keys, runs: coverageStartClaimRepository{}, key: runnerKey, want: "start claim rejected"},
		{name: "signing error", raw: raw, keys: keys, runs: coverageStartClaimRepository{granted: true}, key: protocol.SigningKey{ID: "invalid"}, want: "start grant signing failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reply, err := protocol.DecodeStartClaimReply(startClaimResponse(context.Background(), test.runs, test.keys, test.key, test.raw))
			if err != nil || reply.Error != test.want {
				t.Fatalf("reply = %#v, err = %v", reply, err)
			}
		})
	}
}

func mustJSON(t *testing.T, value protocol.StartClaimPayload) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustEnvelopeBytes(t *testing.T, key protocol.SigningKey, payload []byte) []byte {
	t.Helper()
	envelope := protocol.NewEnvelope(key.ID, payload)
	if err := envelope.Sign(key.Private, protocol.StartClaimSignatureDomain); err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneStartClaim(value protocol.StartClaimPayload, mutate func(*protocol.StartClaimPayload)) []byte {
	mutate(&value)
	return mustJSONWithoutTest(value)
}

func mustJSONWithoutTest(value protocol.StartClaimPayload) []byte {
	raw, _ := json.Marshal(value)
	return raw
}
