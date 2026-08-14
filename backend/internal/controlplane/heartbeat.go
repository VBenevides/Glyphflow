package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

type RunnerHeartbeatRepository interface {
	Heartbeat(context.Context, string, time.Time) error
	MarkStale(context.Context, time.Time) error
}

type RunnerSessionHeartbeatRepository interface {
	HeartbeatWithKey(context.Context, string, string, time.Time, string, []byte) error
}

func RunRunnerHeartbeatMonitor(ctx context.Context, events *queue.JetStream, repository RunnerHeartbeatRepository, staleAfter, checkInterval time.Duration) error {
	if events == nil || repository == nil || staleAfter <= 0 || checkInterval <= 0 {
		return errors.New("runner heartbeat monitor is not configured")
	}
	monitorCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	consumer, err := events.Consumer(monitorCtx, "control-plane-runner-heartbeats", "glyphflow.events.>", 100)
	if err != nil {
		return err
	}
	go func() {
		for monitorCtx.Err() == nil {
			if err := events.ConsumeOne(monitorCtx, consumer, func(ctx context.Context, message queue.Message) error {
				return recordRunnerHeartbeat(ctx, repository, message.Data)
			}); err != nil && monitorCtx.Err() == nil {
				select {
				case <-time.After(time.Second):
				case <-monitorCtx.Done():
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := repository.MarkStale(ctx, now.UTC().Add(-staleAfter)); err != nil {
				return fmt.Errorf("mark stale runners: %w", err)
			}
		}
	}
}

func recordRunnerHeartbeat(ctx context.Context, repository RunnerHeartbeatRepository, raw []byte) error {
	if _, err := protocol.DecodeEnvelope(raw); err == nil {
		return nil
	}
	var heartbeat struct {
		RunnerID  string `json:"runner_id"`
		BootID    string `json:"boot_id"`
		At        string `json:"at"`
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
		EventID   string `json:"event_id"`
	}
	if err := json.Unmarshal(raw, &heartbeat); err != nil {
		return fmt.Errorf("invalid runner heartbeat: %w", err)
	}
	if heartbeat.EventID != "" && heartbeat.BootID == "" {
		return nil
	}
	if strings.TrimSpace(heartbeat.RunnerID) == "" || strings.TrimSpace(heartbeat.BootID) == "" || strings.TrimSpace(heartbeat.At) == "" {
		return errors.New("runner heartbeat fields are required")
	}
	at, err := time.Parse(time.RFC3339Nano, heartbeat.At)
	if err != nil {
		return fmt.Errorf("invalid runner heartbeat timestamp: %w", err)
	}
	if heartbeat.PublicKey != "" {
		publicKey, err := base64.RawStdEncoding.DecodeString(heartbeat.PublicKey)
		if err != nil || len(publicKey) != ed25519.PublicKeySize {
			return errors.New("invalid runner public key")
		}
		if sessionRepository, ok := repository.(RunnerSessionHeartbeatRepository); ok {
			return sessionRepository.HeartbeatWithKey(ctx, heartbeat.RunnerID, heartbeat.BootID, at.UTC(), heartbeat.KeyID, publicKey)
		}
	}
	return repository.Heartbeat(ctx, heartbeat.RunnerID, at.UTC())
}
