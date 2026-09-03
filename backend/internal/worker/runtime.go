package worker

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

const (
	logFlushInterval      = 10 * time.Second
	maxEventLogChunkBytes = 64 * 1024
	controlPlaneKeyID     = "control-plane"
)

type OrderRuntime struct {
	Store            *LocalStore
	Publisher        queue.Publisher
	StartClaimer     StartClaimer
	SecretFetcher    SecretFetcher
	RunnerID         string
	ExecutorBootID   string
	ProcessID        int64
	ControlPublicKey ed25519.PublicKey
	SigningKey       protocol.SigningKey
	Executor         Executor
	Active           *ActiveOrders
	Writer           io.Writer
}

type eventRecord struct {
	typeName  protocol.EventType
	sequence  uint64
	result    string
	errorText string
	exitCode  *int
	channel   string
	metrics   map[string]int64
}

type executionResult struct {
	output   []byte
	exitCode *int
	runErr   error
	streamed bool
	memory   *MemoryStats
}

func (r OrderRuntime) logf(format string, args ...any) {
	writer := r.Writer
	if writer == nil {
		writer = io.Discard
	}
	_, _ = fmt.Fprintf(writer, format, args...)
}

// ponytail: one outbox drain lock is sufficient for this worker process; split by runner only if publishing becomes a bottleneck.
var pendingEventsMu sync.Mutex

type activeOrder struct {
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

type ActiveOrders struct {
	mu     sync.Mutex
	orders map[string]*activeOrder
}

func (a *ActiveOrders) Count() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int64(len(a.orders))
}

func (a *ActiveOrders) put(id string, cancel context.CancelFunc) *activeOrder {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.orders == nil {
		a.orders = map[string]*activeOrder{}
	}
	item := &activeOrder{cancel: cancel}
	a.orders[id] = item
	return item
}
func (a *ActiveOrders) remove(id string) { a.mu.Lock(); delete(a.orders, id); a.mu.Unlock() }
func (a *ActiveOrders) cancel(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	item, ok := a.orders[id]
	if !ok {
		return false
	}
	item.cancelled.Store(true)
	item.cancel()
	return true
}

func (r OrderRuntime) Handle(ctx context.Context, message queue.Message) error {
	if r.Store == nil || r.Publisher == nil || r.StartClaimer == nil || r.RunnerID == "" || len(r.ControlPublicKey) != ed25519.PublicKeySize || len(r.SigningKey.Private) != ed25519.PrivateKeySize {
		return errors.New("worker order runtime is not configured")
	}
	envelope, err := protocol.DecodeEnvelope(message.Data)
	if err != nil {
		return err
	}
	keyring := protocol.Keyring{controlPlaneKeyID: {ID: controlPlaneKeyID, PublicKey: r.ControlPublicKey}}
	if err := keyring.VerifyAt(envelope, protocol.OrderSignatureDomain, time.Now().UTC()); err != nil {
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
	if order.Type == protocol.OrderCancel {
		if err := order.ValidateTime(time.Now().UTC(), time.Second); err != nil {
			return err
		}
		if err := order.ValidateIdentity(r.RunnerID, order.RunID, order.Attempt, order.LeaseToken); err != nil {
			return err
		}
		if err := order.ValidateExecution(); err != nil {
			return err
		}
		if r.Active != nil {
			// Completion can win the cancellation race. The signed, attempt-specific
			// order is then already satisfied and must not be redelivered forever.
			_ = r.Active.cancel(order.TargetOrderID)
		}
		return nil
	}
	return r.handleExecution(ctx, message, order, keyring)
}

func (r OrderRuntime) handleExecution(ctx context.Context, message queue.Message, order protocol.OrderPayload, keyring protocol.Keyring) error {
	payload, err := protocol.VerifyOrder(message.Data, keyring, time.Now().UTC(), r.RunnerID, order.RunID, order.Attempt, order.LeaseToken, time.Second, nil)
	if err != nil {
		return err
	}
	if payload.Type != protocol.OrderExecute {
		return errors.New("unsupported worker order type")
	}
	secretValues, err := r.fetchExecutionSecrets(ctx, payload)
	if err != nil {
		return err
	}
	if secretValues != nil {
		defer func() { clear(secretValues) }()
	}
	payload, proceed, err := r.claimExecution(ctx, message, order, payload)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	taskName := payload.TaskName
	if taskName == "" {
		taskName = payload.TaskID
	}
	r.logf("> %s - Started Task %q v%d - ID %q - Run %q\n", time.Now().UTC().Format("2006-01-02 15:04 MST"), taskName, payload.TaskVersion, payload.TaskID, payload.RunID)
	if err := r.publishEvent(ctx, payload, eventRecord{typeName: protocol.EventStarted, sequence: 2, channel: "state"}); err != nil {
		return err
	}
	executionContext, cancel := context.WithTimeout(ctx, time.Duration(payload.DurationSeconds)*time.Second)
	defer cancel()
	var active *activeOrder
	if r.Active != nil {
		active = r.Active.put(payload.OrderID, cancel)
		defer r.Active.remove(payload.OrderID)
	}
	output, exitCode, runErr, streamed, memory := r.runExecutor(ctx, executionContext, payload, secretValues)
	return r.finishExecution(ctx, executionContext, payload, taskName, active, executionResult{output: output, exitCode: exitCode, runErr: runErr, streamed: streamed, memory: memory})
}

func (r OrderRuntime) fetchExecutionSecrets(ctx context.Context, payload protocol.OrderPayload) (map[string]string, error) {
	if len(payload.SecretRefs) == 0 {
		return nil, nil
	}
	if r.SecretFetcher == nil {
		return nil, ErrSecretDeliveryUnavailable
	}
	secretValues, err := r.SecretFetcher.Fetch(ctx, protocol.SecretDeliveryRequest{Version: protocol.ProtocolVersion, RequestID: payload.OrderID, OrderID: payload.OrderID, RunID: payload.RunID, Attempt: payload.Attempt, LeaseToken: payload.LeaseToken, RunnerID: payload.RunnerID, RunnerSessionID: payload.RunnerSessionID, FencingToken: payload.FencingToken, ExecutionSpecDigest: payload.ExecutionSpecDigest, SecretRefs: payload.SecretRefs, IssuedAt: time.Now().UTC()})
	if err != nil {
		return nil, ErrSecretDeliveryUnavailable
	}
	return secretValues, nil
}

func (r OrderRuntime) claimExecution(ctx context.Context, message queue.Message, order, payload protocol.OrderPayload) (protocol.OrderPayload, bool, error) {
	if err := r.StartClaimer.ClaimStart(ctx, protocol.StartClaimPayload{Version: protocol.ProtocolVersion, RequestID: payload.OrderID, RunID: payload.RunID, RunnerID: payload.RunnerID, RunnerSessionID: payload.RunnerSessionID, LeaseToken: payload.LeaseToken, Attempt: payload.Attempt, FencingToken: payload.FencingToken, ExecutionSpecDigest: payload.ExecutionSpecDigest, IssuedAt: time.Now().UTC()}); err != nil {
		if errors.Is(err, ErrStartRejected) {
			return payload, false, nil
		}
		return payload, false, err
	}
	payload, err := r.Store.AcceptOrder(message.Data, protocol.Keyring{controlPlaneKeyID: {ID: controlPlaneKeyID, PublicKey: r.ControlPublicKey}}, time.Now().UTC(), r.RunnerID, order.RunID, order.Attempt, order.LeaseToken, time.Second)
	if err != nil {
		return payload, false, err
	}
	if err := r.Store.ClaimOrder(payload.OrderID, r.ExecutorBootID, r.ProcessID); err != nil {
		return payload, false, nil
	}
	if err := r.publishEvent(ctx, payload, eventRecord{typeName: protocol.EventAccepted, sequence: 1, channel: "state"}); err != nil {
		return payload, false, err
	}
	if err := r.Store.MarkProcessStarted(payload.OrderID); err != nil {
		return payload, false, err
	}
	return payload, true, nil
}

func (r OrderRuntime) runExecutor(ctx, executionContext context.Context, payload protocol.OrderPayload, secretValues map[string]string) ([]byte, *int, error, bool, *MemoryStats) {
	executor := r.Executor
	if secretValues != nil {
		executor.Environment = make(map[string]string, len(payload.Environment)+len(secretValues))
		for name, value := range payload.Environment {
			executor.Environment[name] = value
		}
		for name, value := range secretValues {
			executor.Environment[name] = value
		}
	} else if payload.Environment != nil {
		executor.Environment = payload.Environment
	}
	memory := &MemoryStats{}
	executor.Metrics = memory
	logSequences := map[string]uint64{"stdout": 0, "stderr": 0}
	streamed := false
	output, exitCode, runErr := executor.RunStreamingWithExitCode(executionContext, payload.Command, payload.WorkingDir, logFlushInterval, func(stream string, chunk []byte) error {
		return r.persistOutputChunks(ctx, payload, logSequences, &streamed, stream, chunk)
	})
	if secretValues != nil {
		clear(executor.Environment)
	}
	return output, exitCode, runErr, streamed, memory
}

func (r OrderRuntime) persistOutputChunks(ctx context.Context, payload protocol.OrderPayload, logSequences map[string]uint64, streamed *bool, stream string, chunk []byte) error {
	if stream != "stdout" && stream != "stderr" {
		return errors.New("executor returned an invalid output stream")
	}
	for len(chunk) > 0 {
		size := len(chunk)
		if size > maxEventLogChunkBytes {
			size = maxEventLogChunkBytes
		}
		logSequences[stream]++
		if err := r.persistEvent(payload, eventRecord{typeName: protocol.EventLogChunk, sequence: logSequences[stream], result: string(chunk[:size]), channel: stream}); err != nil {
			return err
		}
		_ = PublishPendingEvents(ctx, r.Store, r.Publisher, r.RunnerID)
		*streamed = true
		chunk = chunk[size:]
	}
	return nil
}

func (r OrderRuntime) finishExecution(ctx, executionContext context.Context, payload protocol.OrderPayload, taskName string, active *activeOrder, result executionResult) error {
	exitCode, runErr, timedOut := normalizeExecutionResult(executionContext, result.exitCode, result.runErr)
	finishedCode := -1
	if exitCode != nil {
		finishedCode = *exitCode
	}
	r.logf("> %s - Finished Task %q v%d - ID %q - Run %q - %d\n", time.Now().UTC().Format("2006-01-02 15:04 MST"), taskName, payload.TaskVersion, payload.TaskID, payload.RunID, finishedCode)
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	terminalOutput := ""
	if !result.streamed {
		terminalOutput = string(result.output)
	}
	metrics := map[string]int64{"max_memory_bytes": int64(result.memory.MaxBytes), "average_memory_bytes": int64(result.memory.AverageBytes)}
	if err := r.publishEvent(ctx, payload, eventRecord{typeName: terminalEventType(runErr, timedOut, active), sequence: 3, result: terminalOutput, errorText: errorText, exitCode: exitCode, channel: "state", metrics: metrics}); err != nil {
		return err
	}
	return r.Store.FinishOrder(payload.OrderID, terminalState(runErr, active), errorText)
}

func normalizeExecutionResult(ctx context.Context, exitCode *int, runErr error) (*int, error, bool) {
	if exitCode != nil && *exitCode != 0 && runErr == nil {
		runErr = fmt.Errorf("process exited with code %d", *exitCode)
	}
	if exitCode == nil && runErr != nil {
		genericError := 1
		exitCode = &genericError
	}
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if timedOut {
		timeoutCode := 6
		exitCode = &timeoutCode
		runErr = errors.New("Timeout")
	}
	return exitCode, runErr, timedOut
}

func terminalEventType(runErr error, timedOut bool, active *activeOrder) protocol.EventType {
	if active != nil && active.cancelled.Load() {
		return protocol.EventCancelled
	}
	if timedOut {
		return protocol.EventTimedOut
	}
	if runErr != nil {
		return protocol.EventFailed
	}
	return protocol.EventCompleted
}

func terminalState(runErr error, active *activeOrder) string {
	if active != nil && active.cancelled.Load() {
		return "CANCELLED"
	}
	if runErr != nil {
		return "FAILED"
	}
	return "COMPLETED"
}

func (r OrderRuntime) publishEvent(ctx context.Context, order protocol.OrderPayload, event eventRecord) error {
	if err := r.persistEvent(order, event); err != nil {
		return err
	}
	// The event is durable locally; the background publisher retries transient broker failures.
	_ = PublishPendingEvents(ctx, r.Store, r.Publisher, r.RunnerID)
	return nil
}

func (r OrderRuntime) persistEvent(order protocol.OrderPayload, event eventRecord) error {
	eventID := order.OrderID + ":" + string(event.typeName)
	if event.typeName == protocol.EventLogChunk {
		eventID = order.OrderID + ":" + event.channel + ":" + strconv.FormatUint(event.sequence, 10)
	}
	payload, err := protocol.EncodeEventPayload(protocol.EventPayload{Version: protocol.ProtocolVersion, EventID: eventID, OrderID: order.OrderID, RunID: order.RunID, TaskID: order.TaskID, Attempt: order.Attempt, LeaseToken: order.LeaseToken, RunnerID: order.RunnerID, Sequence: event.sequence, ObservedAt: time.Now().UTC(), Type: event.typeName, Result: event.result, Metrics: event.metrics, Error: event.errorText, ExitCode: event.exitCode, RunnerSessionID: order.RunnerSessionID, FencingToken: order.FencingToken, EventChannel: event.channel})
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
	if err := r.Store.PutEvent(OutboxEvent{EventID: eventID, OrderID: order.OrderID, Channel: event.channel, Sequence: int64(event.sequence), EventType: string(event.typeName), Envelope: string(raw)}); err != nil {
		return err
	}
	return nil
}

func PublishPendingEvents(ctx context.Context, store *LocalStore, publisher queue.Publisher, runnerID string) error {
	if store == nil || publisher == nil || runnerID == "" {
		return errors.New("worker event publisher is not configured")
	}
	pendingEventsMu.Lock()
	defer pendingEventsMu.Unlock()
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
	_, err = store.CompactPublishedEvents(24*time.Hour, 1000)
	return err
}
