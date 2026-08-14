package worker

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strconv"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

const (
	logFlushInterval      = 30 * time.Second
	maxEventLogChunkBytes = 64 * 1024
)

type OrderRuntime struct {
	Store            *LocalStore
	Publisher        queue.Publisher
	RunnerID         string
	ExecutorBootID   string
	ProcessID        int64
	ControlPublicKey ed25519.PublicKey
	SigningKey       protocol.SigningKey
	Executor         Executor
}

func (r OrderRuntime) Handle(ctx context.Context, message queue.Message) error {
	if r.Store == nil || r.Publisher == nil || r.RunnerID == "" || len(r.ControlPublicKey) != ed25519.PublicKeySize || len(r.SigningKey.Private) != ed25519.PrivateKeySize {
		return errors.New("worker order runtime is not configured")
	}
	envelope, err := protocol.DecodeEnvelope(message.Data)
	if err != nil {
		return err
	}
	rawPayload, err := envelope.PayloadBytes()
	if err != nil {
		return err
	}
	order, err := protocol.DecodeOrderPayload(rawPayload)
	if err != nil {
		return err
	}
	payload, err := r.Store.AcceptOrder(message.Data, protocol.Keyring{"control-plane": {ID: "control-plane", PublicKey: r.ControlPublicKey}}, time.Now().UTC(), r.RunnerID, order.RunID, order.Attempt, order.LeaseToken, time.Second)
	if err != nil {
		return err
	}
	if payload.Type != protocol.OrderExecute {
		return errors.New("unsupported worker order type")
	}
	if err := r.Store.ClaimOrder(payload.OrderID, r.ExecutorBootID, r.ProcessID); err != nil {
		return nil
	}
	if err := r.publishEvent(ctx, payload, protocol.EventAccepted, 1, "", ""); err != nil {
		return err
	}
	if err := r.Store.MarkProcessStarted(payload.OrderID); err != nil {
		return err
	}
	if err := r.publishEvent(ctx, payload, protocol.EventStarted, 2, "", ""); err != nil {
		return err
	}
	executionContext, cancel := context.WithTimeout(ctx, time.Duration(payload.TimeoutSeconds)*time.Second)
	defer cancel()
	logSequences := map[string]uint64{"stdout": 0, "stderr": 0}
	streamed := false
	output, runErr := r.Executor.RunStreaming(executionContext, payload.Command, payload.WorkingDir, logFlushInterval, func(stream string, chunk []byte) error {
		if stream != "stdout" && stream != "stderr" {
			return errors.New("executor returned an invalid output stream")
		}
		for len(chunk) > 0 {
			size := len(chunk)
			if size > maxEventLogChunkBytes {
				size = maxEventLogChunkBytes
			}
			logSequences[stream]++
			if err := r.persistEvent(payload, protocol.EventLogChunk, logSequences[stream], string(chunk[:size]), "", stream); err != nil {
				return err
			}
			_ = PublishPendingEvents(ctx, r.Store, r.Publisher, r.RunnerID)
			streamed = true
			chunk = chunk[size:]
		}
		return nil
	})
	eventType := protocol.EventCompleted
	if runErr != nil {
		eventType = protocol.EventFailed
	}
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	terminalOutput := ""
	if !streamed {
		terminalOutput = string(output)
	}
	if err := r.publishEvent(ctx, payload, eventType, 3, terminalOutput, errorText); err != nil {
		return err
	}
	state := "COMPLETED"
	if runErr != nil {
		state = "FAILED"
	}
	return r.Store.FinishOrder(payload.OrderID, state, errorText)
}

func (r OrderRuntime) publishEvent(ctx context.Context, order protocol.OrderPayload, eventType protocol.EventType, sequence uint64, result, eventError string) error {
	if err := r.persistEvent(order, eventType, sequence, result, eventError, "state"); err != nil {
		return err
	}
	// The event is durable locally; the background publisher retries transient broker failures.
	_ = PublishPendingEvents(ctx, r.Store, r.Publisher, r.RunnerID)
	return nil
}

func (r OrderRuntime) persistEvent(order protocol.OrderPayload, eventType protocol.EventType, sequence uint64, result, eventError, eventChannel string) error {
	eventID := order.OrderID + ":" + string(eventType)
	if eventType == protocol.EventLogChunk {
		eventID = order.OrderID + ":" + eventChannel + ":" + strconv.FormatUint(sequence, 10)
	}
	event := protocol.EventPayload{Version: protocol.ProtocolVersion, EventID: eventID, OrderID: order.OrderID, RunID: order.RunID, TaskID: order.TaskID, Attempt: order.Attempt, LeaseToken: order.LeaseToken, RunnerID: order.RunnerID, Sequence: sequence, ObservedAt: time.Now().UTC(), Type: eventType, Result: result, Error: eventError, RunnerSessionID: order.RunnerSessionID, FencingToken: order.FencingToken, EventChannel: eventChannel}
	payload, err := protocol.EncodeEventPayload(event)
	if err != nil {
		return err
	}
	envelope, err := r.SigningKey.SignEvent(payload)
	if err != nil {
		return err
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := r.Store.PutEvent(OutboxEvent{EventID: event.EventID, OrderID: order.OrderID, Channel: eventChannel, Sequence: int64(sequence), EventType: string(eventType), Envelope: string(raw)}); err != nil {
		return err
	}
	return nil
}

func PublishPendingEvents(ctx context.Context, store *LocalStore, publisher queue.Publisher, runnerID string) error {
	if store == nil || publisher == nil || runnerID == "" {
		return errors.New("worker event publisher is not configured")
	}
	pendingEvents, err := store.PendingEvents(100)
	if err != nil {
		return err
	}
	for _, pending := range pendingEvents {
		if err := publisher.Publish(ctx, queue.Message{Subject: queue.Subject("events", runnerID), ID: pending.EventID, Data: []byte(pending.Envelope)}); err != nil {
			return err
		}
		if err := store.MarkEventPublished(pending.EventID); err != nil {
			return err
		}
	}
	return nil
}
