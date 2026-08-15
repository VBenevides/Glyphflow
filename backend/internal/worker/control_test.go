package worker

import (
	"context"
	"crypto/ed25519"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

func TestApplyRunnerControlUpdatesCapacity(t *testing.T) {
	key, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := protocol.EncodeRunnerControlPayload(protocol.RunnerControlPayload{Version: protocol.ProtocolVersion, Type: protocol.RunnerControlCapacity, RunnerID: "runner-1", Capacity: 42, IssuedAt: time.Now().UTC()})
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
	var capacity atomic.Int64
	capacity.Store(1)
	if err := ApplyRunnerControl(context.Background(), queue.Message{Subject: queue.Subject("control", "runner-1"), Data: raw}, "runner-1", ed25519.PublicKey(key.Public.PublicKey), &capacity); err != nil {
		t.Fatal(err)
	}
	if capacity.Load() != 42 {
		t.Fatalf("capacity = %d, want 42", capacity.Load())
	}
}
