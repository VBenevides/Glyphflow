package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

type heartbeatRepository struct {
	id string
	at time.Time
}

func TestRecordRunnerHeartbeatIgnoresRunnerEventEnvelope(t *testing.T) {
	repository := &heartbeatRepository{}
	raw, err := protocol.EncodeEnvelope(protocol.NewEnvelope("runner:1", []byte(`{"event_id":"event-1"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if err := recordRunnerHeartbeat(context.Background(), repository, raw); err != nil {
		t.Fatal(err)
	}
	if repository.id != "" {
		t.Fatalf("event envelope recorded as heartbeat for %q", repository.id)
	}
}

func (r *heartbeatRepository) Heartbeat(_ context.Context, id string, at time.Time) error {
	r.id, r.at = id, at
	return nil
}

func (r *heartbeatRepository) MarkStale(context.Context, time.Time) error { return nil }

func TestRecordRunnerHeartbeat(t *testing.T) {
	repository := &heartbeatRepository{}
	want := time.Date(2026, 8, 14, 15, 0, 0, 123, time.UTC)
	if err := recordRunnerHeartbeat(context.Background(), repository, []byte(`{"runner_id":"runner-1","boot_id":"boot-1","at":"2026-08-14T15:00:00.000000123Z"}`)); err != nil {
		t.Fatal(err)
	}
	if repository.id != "runner-1" || !repository.at.Equal(want) {
		t.Fatalf("heartbeat = %q %s", repository.id, repository.at)
	}
}

func TestRecordRunnerHeartbeatRejectsInvalidPayload(t *testing.T) {
	repository := &heartbeatRepository{}
	if err := recordRunnerHeartbeat(context.Background(), repository, []byte(`{"runner_id":"runner-1"}`)); err == nil {
		t.Fatal("invalid heartbeat was accepted")
	}
}
