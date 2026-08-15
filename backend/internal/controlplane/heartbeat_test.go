package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

type heartbeatRepository struct {
	id  string
	at  time.Time
	key protocol.SigningKey
}

func TestRecordRunnerHeartbeatIgnoresRunnerEventEnvelope(t *testing.T) {
	repository := &heartbeatRepository{}
	raw, err := protocol.EncodeEnvelope(protocol.NewEnvelope("runner:1", []byte(`{"event_id":"event-1"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if err := recordRunnerHeartbeat(context.Background(), repository, raw); err == nil {
		t.Fatal("unsigned heartbeat envelope was accepted")
	}
	if repository.id != "" {
		t.Fatalf("event envelope recorded as heartbeat for %q", repository.id)
	}
}

func (r *heartbeatRepository) Heartbeat(_ context.Context, id string, at time.Time) error {
	r.id, r.at = id, at
	return nil
}

func (r *heartbeatRepository) FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error) {
	return r.key.Public.PublicKey, nil
}

func (r *heartbeatRepository) HeartbeatWithKey(_ context.Context, id, _ string, at time.Time, _ string, _ []byte) error {
	r.id, r.at = id, at
	return nil
}

func (r *heartbeatRepository) MarkStale(context.Context, time.Time) error { return nil }

func TestRecordRunnerHeartbeat(t *testing.T) {
	repository := &heartbeatRepository{}
	key, err := protocol.GenerateSigningKey("runner:1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	repository.key = key
	payload, _ := json.Marshal(map[string]string{"runner_id": "runner-1", "boot_id": "boot-1", "at": time.Now().UTC().Format(time.RFC3339Nano)})
	envelope, err := key.SignEvent(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordRunnerHeartbeat(context.Background(), repository, raw); err != nil {
		t.Fatal(err)
	}
	if repository.id != "runner-1" || repository.at.IsZero() {
		t.Fatalf("heartbeat = %q %s", repository.id, repository.at)
	}
}

func TestRecordRunnerHeartbeatRejectsInvalidPayload(t *testing.T) {
	repository := &heartbeatRepository{}
	if err := recordRunnerHeartbeat(context.Background(), repository, []byte(`{"runner_id":"runner-1"}`)); err == nil {
		t.Fatal("invalid heartbeat was accepted")
	}
}
