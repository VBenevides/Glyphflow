package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type RunnerHeartbeatRepository interface {
	Heartbeat(context.Context, string, time.Time) error
	MarkStale(context.Context, time.Time) error
}

type RunnerSessionHeartbeatRepository interface {
	HeartbeatWithKey(context.Context, string, string, time.Time, string, []byte) error
}

type RunnerCapacityHeartbeatRepository interface {
	HeartbeatWithKeyAndCapacity(context.Context, string, string, time.Time, int, string, []byte) error
}

type RunnerResourceMetricsHeartbeatRepository interface {
	HeartbeatWithKeyAndCapacityAndMetrics(context.Context, string, string, time.Time, int, store.RunnerMetricsSample, string, []byte) error
}

func RunRunnerHeartbeatMonitor(ctx context.Context, events queue.EventStream, repository RunnerHeartbeatRepository, staleAfter, checkInterval time.Duration) error {
	if events == nil || repository == nil || staleAfter <= 0 || checkInterval <= 0 {
		return errors.New("runner heartbeat monitor is not configured")
	}
	monitorCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		for monitorCtx.Err() == nil {
			if err := events.ConsumeSubject(monitorCtx, "control-plane-runner-heartbeats", "glyphflow.heartbeats.>", 100, func(ctx context.Context, message queue.Message) error {
				return recordRunnerHeartbeatForSubject(ctx, repository, message.Subject, message.Data)
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

func recordRunnerHeartbeatForSubject(ctx context.Context, repository RunnerHeartbeatRepository, subject string, raw []byte) error {
	envelope, err := protocol.DecodeEnvelope(raw)
	if err != nil {
		return errors.New("runner heartbeat must be signed")
	}
	payload, err := envelope.PayloadBytes()
	if err != nil {
		return err
	}
	var heartbeat struct {
		RunnerID         string   `json:"runner_id"`
		BootID           string   `json:"boot_id"`
		At               string   `json:"at"`
		KeyID            string   `json:"key_id"`
		Capacity         int      `json:"capacity"`
		CPUPercent       *float64 `json:"cpu_percent"`
		MemoryPercent    *float64 `json:"memory_percent"`
		MemoryUsedBytes  *int64   `json:"memory_used_bytes"`
		MemoryTotalBytes *int64   `json:"memory_total_bytes"`
	}
	if err := json.Unmarshal(payload, &heartbeat); err != nil {
		return fmt.Errorf("invalid runner heartbeat: %w", err)
	}
	if strings.TrimSpace(heartbeat.RunnerID) == "" || strings.TrimSpace(heartbeat.BootID) == "" || strings.TrimSpace(heartbeat.At) == "" {
		return errors.New("runner heartbeat fields are required")
	}
	if subject != queue.Subject("heartbeats", heartbeat.RunnerID) {
		return errors.New("runner heartbeat subject does not match runner")
	}
	keyRepository, ok := repository.(interface {
		FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error)
	})
	if !ok {
		return errors.New("runner heartbeat key repository is unavailable")
	}
	publicKey, err := keyRepository.FindPublicKey(ctx, heartbeat.RunnerID, envelope.KeyID)
	if err != nil {
		return errors.New("runner heartbeat key is not enrolled")
	}
	if err := envelope.VerifyEvent(publicKey); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, heartbeat.At)
	if err != nil {
		return fmt.Errorf("invalid runner heartbeat timestamp: %w", err)
	}
	now := time.Now().UTC()
	if at.Before(now.Add(-30*time.Second)) || at.After(now.Add(30*time.Second)) {
		return errors.New("runner heartbeat timestamp is outside the allowed window")
	}
	var sample *store.RunnerMetricsSample
	if heartbeat.CPUPercent != nil || heartbeat.MemoryPercent != nil || heartbeat.MemoryUsedBytes != nil || heartbeat.MemoryTotalBytes != nil {
		if heartbeat.CPUPercent == nil || heartbeat.MemoryPercent == nil || heartbeat.MemoryUsedBytes == nil || heartbeat.MemoryTotalBytes == nil {
			return errors.New("runner heartbeat metrics are incomplete")
		}
		sample = &store.RunnerMetricsSample{CPUPercent: *heartbeat.CPUPercent, MemoryPercent: *heartbeat.MemoryPercent, MemoryUsedBytes: *heartbeat.MemoryUsedBytes, MemoryTotalBytes: *heartbeat.MemoryTotalBytes}
	}
	if sample != nil {
		if sessionRepository, ok := repository.(RunnerResourceMetricsHeartbeatRepository); ok {
			return sessionRepository.HeartbeatWithKeyAndCapacityAndMetrics(ctx, heartbeat.RunnerID, heartbeat.BootID, at.UTC(), heartbeat.Capacity, *sample, envelope.KeyID, publicKey)
		}
		return errors.New("runner heartbeat metrics repository is unavailable")
	}
	if sessionRepository, ok := repository.(RunnerCapacityHeartbeatRepository); ok {
		return sessionRepository.HeartbeatWithKeyAndCapacity(ctx, heartbeat.RunnerID, heartbeat.BootID, at.UTC(), heartbeat.Capacity, envelope.KeyID, publicKey)
	}
	if sessionRepository, ok := repository.(RunnerSessionHeartbeatRepository); ok {
		return sessionRepository.HeartbeatWithKey(ctx, heartbeat.RunnerID, heartbeat.BootID, at.UTC(), envelope.KeyID, publicKey)
	}
	return errors.New("runner heartbeat session repository is unavailable")
}

func isRunnerHeartbeat(raw []byte) bool {
	envelope, err := protocol.DecodeEnvelope(raw)
	if err != nil {
		return false
	}
	payload, err := envelope.PayloadBytes()
	if err != nil {
		return false
	}
	var heartbeat struct {
		RunnerID string `json:"runner_id"`
		BootID   string `json:"boot_id"`
		At       string `json:"at"`
	}
	return json.Unmarshal(payload, &heartbeat) == nil && heartbeat.RunnerID != "" && heartbeat.BootID != "" && heartbeat.At != ""
}
