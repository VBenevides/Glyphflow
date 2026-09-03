package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type dispatchErrorRepository struct {
	coverageDispatchRepository
	claimWaitingErr, claimCancellingErr, retryErr, markErr error
}

func (r dispatchErrorRepository) ClaimWaiting(ctx context.Context, build func(store.DispatchCandidate) ([]byte, error)) (store.DispatchCandidate, bool, error) {
	if r.claimWaitingErr != nil {
		return store.DispatchCandidate{}, false, r.claimWaitingErr
	}
	return r.coverageDispatchRepository.ClaimWaiting(ctx, build)
}

func (r dispatchErrorRepository) ClaimCancelling(ctx context.Context, build func(store.CancellationCandidate) ([]byte, error)) (store.CancellationCandidate, bool, error) {
	if r.claimCancellingErr != nil {
		return store.CancellationCandidate{}, false, r.claimCancellingErr
	}
	return r.coverageDispatchRepository.ClaimCancelling(ctx, build)
}

func (r dispatchErrorRepository) RetryDispatch(context.Context, string, error) error {
	return r.retryErr
}
func (r dispatchErrorRepository) MarkDispatchPublished(context.Context, string) error {
	return r.markErr
}

type failingPublisher struct{ err error }

func (p failingPublisher) Publish(context.Context, queue.Message) error { return p.err }

func TestDispatchAndPublishErrorBranches(t *testing.T) {
	key, err := protocol.GenerateSigningKey("control", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := dispatchWaiting(ctx, &coverageEventStream{}, &dispatchErrorRepository{claimWaitingErr: errors.New("claim waiting")}, key); err == nil {
		t.Fatal("waiting claim error was ignored")
	}
	if err := dispatchCancellations(ctx, &coverageEventStream{}, &dispatchErrorRepository{claimCancellingErr: errors.New("claim cancelling")}, key); err == nil {
		t.Fatal("cancellation claim error was ignored")
	}
	item := store.DispatchOutboxRecord{MessageID: "message", Subject: "subject", Envelope: []byte("envelope")}
	if err := publishPending(ctx, failingPublisher{err: errors.New("publish")}, &dispatchErrorRepository{coverageDispatchRepository: coverageDispatchRepository{pending: []store.DispatchOutboxRecord{item}}}); err != nil {
		t.Fatal(err)
	}
	if err := publishPending(ctx, failingPublisher{err: errors.New("publish")}, &dispatchErrorRepository{coverageDispatchRepository: coverageDispatchRepository{pending: []store.DispatchOutboxRecord{item}}, retryErr: errors.New("retry")}); err == nil {
		t.Fatal("retry error was ignored")
	}
	if err := publishPending(ctx, &coverageEventStream{}, &dispatchErrorRepository{coverageDispatchRepository: coverageDispatchRepository{pending: []store.DispatchOutboxRecord{item}}, markErr: errors.New("mark")}); err == nil {
		t.Fatal("mark error was ignored")
	}
}
