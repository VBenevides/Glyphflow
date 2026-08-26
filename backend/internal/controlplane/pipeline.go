package controlplane

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

type TaskRequest struct {
	OrderID    string
	RunID      string
	TaskID     string
	RunnerID   string
	LeaseToken string
	Command    []string
	WorkingDir string
	Timeout    time.Duration
	MaxOutput  int
}

type RunResult struct {
	State  string
	Output []byte
}

type Pipeline struct {
	ControlPrivate ed25519.PrivateKey
	WorkerPrivate  ed25519.PrivateKey
	Queue          *queue.Memory
	Store          *worker.LocalStore
	Executor       worker.Executor
}

func (p *Pipeline) Execute(ctx context.Context, request TaskRequest) (RunResult, error) {
	if p.Queue == nil || p.Store == nil || len(p.ControlPrivate) != ed25519.PrivateKeySize || len(p.WorkerPrivate) != ed25519.PrivateKeySize {
		return RunResult{}, errors.New("pipeline is not configured")
	}
	if request.Timeout <= 0 {
		return RunResult{}, errors.New("task timeout is required")
	}
	now := time.Now().UTC()
	order := protocol.OrderPayload{
		Version: protocol.ProtocolVersion, OrderID: request.OrderID, RunID: request.RunID, TaskID: request.TaskID,
		Attempt: 1, LeaseToken: request.LeaseToken, RunnerID: request.RunnerID, RunnerSessionID: request.RunnerID + "-session", IssuedAt: now,
		NotBefore: now, ExpiresAt: now.Add(request.Timeout), Type: protocol.OrderExecute,
		Command: request.Command, WorkingDir: request.WorkingDir, TimeoutSeconds: uint32((request.Timeout + time.Second - 1) / time.Second),
		Limits: protocol.ResourceLimits{MaxOutputBytes: uint64(request.MaxOutput)},
	}
	orderBytes, err := protocol.EncodeOrderPayload(order)
	if err != nil {
		return RunResult{}, err
	}
	envelope := protocol.NewEnvelope("control-plane", orderBytes)
	if err := envelope.SignOrder(p.ControlPrivate); err != nil {
		return RunResult{}, err
	}
	rawOrder, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		return RunResult{}, err
	}
	if err := p.Queue.Publish(ctx, queue.Message{Subject: queue.Subject("orders", request.RunnerID), ID: request.OrderID, Data: rawOrder}); err != nil {
		return RunResult{}, err
	}
	message, err := p.Queue.Consume(ctx)
	if err != nil {
		return RunResult{}, err
	}
	verified, err := p.Store.AcceptOrder(message.Data, protocol.Keyring{"control-plane": {ID: "control-plane", PublicKey: p.ControlPrivate.Public().(ed25519.PublicKey)}}, now, request.RunnerID, request.RunID, 1, request.LeaseToken, time.Second)
	if err != nil {
		return RunResult{}, err
	}
	executionContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	executor := p.Executor
	if executor.MaxOutputBytes == 0 {
		executor.MaxOutputBytes = request.MaxOutput
	}
	output, runErr := executor.Run(executionContext, verified.Command, verified.WorkingDir)
	state := protocol.EventCompleted
	if runErr != nil {
		state = protocol.EventFailed
	}
	event := protocol.EventPayload{Version: protocol.ProtocolVersion, EventID: request.OrderID + ":1", OrderID: verified.OrderID, RunID: verified.RunID, TaskID: verified.TaskID, Attempt: verified.Attempt, LeaseToken: verified.LeaseToken, RunnerID: verified.RunnerID, Sequence: 1, ObservedAt: time.Now().UTC(), Type: state, Result: string(output)}
	eventBytes, err := protocol.EncodeEventPayload(event)
	if err != nil {
		return RunResult{}, err
	}
	eventEnvelope := protocol.NewEnvelope("worker", eventBytes)
	if err := eventEnvelope.SignEvent(p.WorkerPrivate); err != nil {
		return RunResult{}, err
	}
	rawEvent, err := protocol.EncodeEnvelope(eventEnvelope)
	if err != nil {
		return RunResult{}, err
	}
	if _, err := protocol.VerifyEvent(rawEvent, protocol.Keyring{"worker": {ID: "worker", PublicKey: p.WorkerPrivate.Public().(ed25519.PublicKey)}}, time.Now().UTC(), request.RunnerID, request.RunID, 1, request.LeaseToken, 1, time.Second, nil); err != nil {
		return RunResult{}, err
	}
	if runErr != nil {
		return RunResult{State: string(state), Output: output}, runErr
	}
	return RunResult{State: string(state), Output: output}, nil
}
