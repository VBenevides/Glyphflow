package worker

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sync/atomic"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

func ApplyRunnerControl(_ context.Context, message queue.Message, runnerID string, publicKey ed25519.PublicKey, capacity *atomic.Int64) error {
	if message.Subject != queue.Subject("control", runnerID) || len(publicKey) != ed25519.PublicKeySize || capacity == nil {
		return errors.New("runner control is not configured")
	}
	envelope, err := protocol.DecodeEnvelope(message.Data)
	if err != nil {
		return err
	}
	if err := envelope.VerifyEvent(publicKey); err != nil {
		return err
	}
	payload, err := envelope.PayloadBytes()
	if err != nil {
		return err
	}
	control, err := protocol.DecodeRunnerControlPayload(payload)
	if err != nil {
		return err
	}
	if control.RunnerID != runnerID {
		return errors.New("runner control does not match runner")
	}
	capacity.Store(int64(control.Capacity))
	return nil
}
