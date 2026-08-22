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

type DispatchRepository interface {
	ClaimWaiting(context.Context, func(store.DispatchCandidate) ([]byte, error)) (store.DispatchCandidate, bool, error)
	ClaimCancelling(context.Context, func(store.CancellationCandidate) ([]byte, error)) (store.CancellationCandidate, bool, error)
	ReconcileTimedOutDispatches(context.Context, time.Time) error
	ReconcileStaleCancellations(context.Context, time.Time) error
	PendingDispatch(context.Context, int) ([]store.DispatchOutboxRecord, error)
	MarkDispatchPublished(context.Context, string) error
	RetryDispatch(context.Context, string, error) error
	ApplyRunEvent(context.Context, store.RunEventInput) error
}

const cancellationReconciliationDelay = 30 * time.Second

type RunnerKeyRepository interface {
	FindPublicKey(context.Context, string, string) (ed25519.PublicKey, error)
}

func RunDispatcher(ctx context.Context, events queue.EventStream, runs DispatchRepository, keys RunnerKeyRepository, signingKey protocol.SigningKey, pollInterval time.Duration) error {
	if events == nil || runs == nil || keys == nil || len(signingKey.Private) != ed25519.PrivateKeySize || pollInterval <= 0 {
		return errors.New("run dispatcher is not configured")
	}
	eventErrors := make(chan error, 1)
	go func() {
		for ctx.Err() == nil {
			if err := events.ConsumeSubject(ctx, "control-plane-run-events", "glyphflow.events.>", 100, func(handlerCtx context.Context, message queue.Message) error {
				return applyRunnerEvent(handlerCtx, keys, runs, message)
			}); err != nil && ctx.Err() == nil {
				select {
				case eventErrors <- err:
				default:
				}
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-eventErrors:
			if err != nil {
				// A malformed or temporarily unverifiable event is NAKed by the
				// queue consumer; the dispatcher continues serving other runs.
			}
		case <-ticker.C:
			if err := dispatchWaiting(ctx, events, runs, signingKey); err != nil {
				return err
			}
			if err := dispatchCancellations(ctx, events, runs, signingKey); err != nil {
				return err
			}
			if err := runs.ReconcileTimedOutDispatches(ctx, time.Now().UTC()); err != nil {
				return err
			}
			if err := runs.ReconcileStaleCancellations(ctx, time.Now().UTC().Add(-cancellationReconciliationDelay)); err != nil {
				return err
			}
			if err := publishPending(ctx, events, runs); err != nil {
				return err
			}
		}
	}
}

func dispatchWaiting(ctx context.Context, events queue.Publisher, runs DispatchRepository, signingKey protocol.SigningKey) error {
	for range 100 {
		candidate, claimed, err := runs.ClaimWaiting(ctx, func(candidate store.DispatchCandidate) ([]byte, error) {
			payload := protocol.OrderPayload{
				Version: protocol.ProtocolVersion, OrderID: candidate.AttemptID, RunID: candidate.RunID,
				TaskID: candidate.TaskID, TaskName: candidate.TaskName, TaskVersion: uint32(candidate.TaskVersion), Attempt: uint32(candidate.AttemptNumber), LeaseToken: candidate.LeaseToken,
				RunnerID: candidate.RunnerID, IssuedAt: time.Now().UTC(), NotBefore: time.Now().UTC(),
				ExpiresAt: candidate.LeaseNotAfter, Type: protocol.OrderExecute, Command: candidate.Command,
				WorkingDir: candidate.WorkingDirectory, TimeoutSeconds: uint32(candidate.TimeoutSeconds),
				Limits: protocol.ResourceLimits{MaxOutputBytes: uint64(candidate.MaxOutputBytes)}, Issuer: signingKey.ID,
				Recipient: candidate.RunnerID, RunnerSessionID: candidate.RunnerSessionID,
				FencingToken: uint64(candidate.FencingToken), LeaseNotAfter: candidate.LeaseNotAfter,
				ExecutionSpecDigest: candidate.ExecutionSpecDigest, Environment: candidate.Environment,
				SecretRefs: candidate.SecretRefs, Resources: candidate.Resources,
			}
			rawPayload, err := protocol.EncodeOrderPayload(payload)
			if err != nil {
				return nil, err
			}
			envelope, err := signingKey.SignOrder(rawPayload)
			if err != nil {
				return nil, err
			}
			return protocol.EncodeEnvelope(envelope)
		})
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		fmt.Printf("%s - Dispatched Run %q - Pool %q - Runner %q\n", time.Now().UTC().Format("2006-01-02 15:04 MST"), candidate.RunID, candidate.Pool, candidate.RunnerID)
	}
	return nil
}

func dispatchCancellations(ctx context.Context, events queue.Publisher, runs DispatchRepository, signingKey protocol.SigningKey) error {
	for range 100 {
		candidate, claimed, err := runs.ClaimCancelling(ctx, func(candidate store.CancellationCandidate) ([]byte, error) {
			now := time.Now().UTC()
			payload := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: candidate.AttemptID,
				TargetOrderID: candidate.AttemptID, RunID: candidate.RunID, TaskID: candidate.TaskID,
				Attempt: uint32(candidate.AttemptNumber), LeaseToken: candidate.LeaseToken, RunnerID: candidate.RunnerID,
				IssuedAt: now, NotBefore: now, ExpiresAt: candidate.LeaseNotAfter, Type: protocol.OrderCancel,
				Issuer: signingKey.ID, Recipient: candidate.RunnerID, RunnerSessionID: candidate.RunnerSessionID,
				FencingToken: uint64(candidate.FencingToken), LeaseNotAfter: candidate.LeaseNotAfter,
				ExecutionSpecDigest: "cancel:" + candidate.Reason, Command: []string{"true"}, WorkingDir: ".", TimeoutSeconds: 1}
			raw, err := protocol.EncodeOrderPayload(payload)
			if err != nil {
				return nil, err
			}
			envelope, err := signingKey.SignOrder(raw)
			if err != nil {
				return nil, err
			}
			encoded, err := protocol.EncodeEnvelope(envelope)
			if err != nil {
				return nil, err
			}
			return encoded, nil
		})
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		_ = candidate
	}
	return nil
}

func publishPending(ctx context.Context, events queue.Publisher, runs DispatchRepository) error {
	items, err := runs.PendingDispatch(ctx, 100)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := events.Publish(ctx, queue.Message{Subject: item.Subject, ID: item.MessageID, Data: item.Envelope}); err != nil {
			if retryErr := runs.RetryDispatch(ctx, item.MessageID, err); retryErr != nil {
				return retryErr
			}
			continue
		}
		if err := runs.MarkDispatchPublished(ctx, item.MessageID); err != nil {
			return err
		}
	}
	return nil
}

func applyRunnerEvent(ctx context.Context, keys RunnerKeyRepository, runs DispatchRepository, message queue.Message) error {
	runnerID := strings.TrimPrefix(message.Subject, "glyphflow.events.")
	if runnerID == message.Subject || runnerID == "" {
		return errors.New("runner event subject is invalid")
	}
	var heartbeat struct {
		BootID string `json:"boot_id"`
		At     string `json:"at"`
	}
	if json.Unmarshal(message.Data, &heartbeat) == nil && heartbeat.BootID != "" && heartbeat.At != "" {
		return nil
	}
	envelope, err := protocol.DecodeEnvelope(message.Data)
	if err != nil {
		return err
	}
	rawPayload, err := envelope.PayloadBytes()
	if err != nil {
		return err
	}
	payload, err := protocol.DecodeEventPayload(rawPayload)
	if err != nil {
		return err
	}
	publicKey, err := keys.FindPublicKey(ctx, runnerID, envelope.KeyID)
	if err != nil {
		return err
	}
	if _, err := protocol.VerifyEvent(message.Data, protocol.Keyring{envelope.KeyID: {ID: envelope.KeyID, PublicKey: publicKey}}, time.Now().UTC(), payload.RunnerID, payload.RunID, payload.Attempt, payload.LeaseToken, payload.Sequence, 30*time.Second, nil); err != nil {
		return err
	}
	return runs.ApplyRunEvent(ctx, store.RunEventInput{
		EventID: payload.EventID, OrderID: payload.OrderID, RunID: payload.RunID, TaskID: payload.TaskID,
		RunnerID: payload.RunnerID, RunnerSessionID: payload.RunnerSessionID, LeaseToken: payload.LeaseToken,
		EventType: string(payload.Type), Subject: message.Subject, Error: payload.Error, Result: payload.Result, Metrics: payload.Metrics,
		EventChannel: payload.EventChannel, ExitCode: payload.ExitCode,
		Attempt: int64(payload.Attempt), Sequence: int64(payload.Sequence), FencingToken: int64(payload.FencingToken),
		ReportedAt: payload.ObservedAt, Envelope: message.Data,
	})
}
