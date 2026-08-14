package worker

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
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
	output, runErr := r.Executor.Run(executionContext, payload.Command, payload.WorkingDir)
	eventType := protocol.EventCompleted
	if runErr != nil {
		eventType = protocol.EventFailed
	}
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	if err := r.publishEvent(ctx, payload, eventType, 3, string(output), errorText); err != nil {
		return err
	}
	state := "COMPLETED"
	if runErr != nil {
		state = "FAILED"
	}
	return r.Store.FinishOrder(payload.OrderID, state, errorText)
}

func (r OrderRuntime) publishEvent(ctx context.Context, order protocol.OrderPayload, eventType protocol.EventType, sequence uint64, result, eventError string) error {
	event := protocol.EventPayload{Version: protocol.ProtocolVersion, EventID: order.OrderID + ":" + string(eventType), OrderID: order.OrderID, RunID: order.RunID, TaskID: order.TaskID, Attempt: order.Attempt, LeaseToken: order.LeaseToken, RunnerID: order.RunnerID, Sequence: sequence, ObservedAt: time.Now().UTC(), Type: eventType, Result: result, Error: eventError, RunnerSessionID: order.RunnerSessionID, FencingToken: order.FencingToken}
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
	if err := r.Store.PutEvent(OutboxEvent{EventID: event.EventID, OrderID: order.OrderID, Channel: "events", Sequence: int64(sequence), EventType: string(eventType), Envelope: string(raw)}); err != nil {
		return err
	}
	return PublishPendingEvents(ctx, r.Store, r.Publisher, r.RunnerID)
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
