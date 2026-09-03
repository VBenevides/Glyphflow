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
)

type heartbeatMinimalRepository struct{}

func (heartbeatMinimalRepository) Heartbeat(context.Context, string, time.Time) error { return nil }
func (heartbeatMinimalRepository) MarkStale(context.Context, time.Time) error         { return nil }

type heartbeatCapacityOnlyRepository struct{ heartbeatMinimalRepository }

func (heartbeatCapacityOnlyRepository) HeartbeatWithKeyAndCapacity(context.Context, string, string, time.Time, int, string, []byte) error {
	return nil
}

type heartbeatSessionOnlyRepository struct{ heartbeatMinimalRepository }

func (heartbeatSessionOnlyRepository) HeartbeatWithKey(context.Context, string, string, time.Time, string, []byte) error {
	return nil
}

type heartbeatKeyErrorRepository struct{ heartbeatMinimalRepository }

func (heartbeatKeyErrorRepository) FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error) {
	return nil, errors.New("key unavailable")
}

func TestHeartbeatPersistenceAndValidationEdges(t *testing.T) {
	key, err := protocol.GenerateSigningKey("runner:edges", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	heartbeat := runnerHeartbeat{RunnerID: "runner", BootID: "boot", At: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := persistRunnerHeartbeat(context.Background(), heartbeatMinimalRepository{}, heartbeat, time.Now().UTC(), key.ID, key.Public.PublicKey); err == nil {
		t.Fatal("heartbeat without session repository accepted")
	}
	cpu, memory, used, total := 1.0, 2.0, int64(3), int64(4)
	heartbeat.CPUPercent, heartbeat.MemoryPercent = &cpu, &memory
	heartbeat.MemoryUsedBytes, heartbeat.MemoryTotalBytes = &used, &total
	if err := persistRunnerHeartbeat(context.Background(), heartbeatMinimalRepository{}, heartbeat, time.Now().UTC(), key.ID, key.Public.PublicKey); err == nil {
		t.Fatal("metrics without metrics repository accepted")
	}
	if err := persistRunnerHeartbeat(context.Background(), heartbeatCapacityOnlyRepository{}, runnerHeartbeat{}, time.Now().UTC(), key.ID, key.Public.PublicKey); err != nil {
		t.Fatal(err)
	}
	if err := persistRunnerHeartbeat(context.Background(), heartbeatSessionOnlyRepository{}, runnerHeartbeat{}, time.Now().UTC(), key.ID, key.Public.PublicKey); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(heartbeat)
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
	if err := recordRunnerHeartbeatForSubject(context.Background(), heartbeatKeyErrorRepository{}, queue.Subject("heartbeats", heartbeat.RunnerID), raw); err == nil {
		t.Fatal("heartbeat key lookup error was ignored")
	}
	if _, err := heartbeatMetrics(runnerHeartbeat{CPUPercent: &cpu, MemoryPercent: &memory, MemoryUsedBytes: &used}); err == nil {
		t.Fatal("incomplete heartbeat metrics accepted")
	}
}
