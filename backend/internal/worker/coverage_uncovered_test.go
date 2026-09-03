package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

type coveragePublishRecorder struct {
	messages []queue.Message
	err      error
	after    func()
}

func (p *coveragePublishRecorder) Publish(_ context.Context, message queue.Message) error {
	if p.after != nil {
		p.after()
	}
	if p.err != nil {
		return p.err
	}
	p.messages = append(p.messages, message)
	return nil
}

func TestPublishPendingEventsCoversDrainAndFailurePaths(t *testing.T) {
	if err := PublishPendingEvents(context.Background(), nil, nil, ""); err == nil {
		t.Fatal("unconfigured publisher accepted")
	}

	store, err := OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PutEvent(OutboxEvent{EventID: "event-1", OrderID: "order-1", Channel: "state", Sequence: 1, Envelope: "raw"}); err != nil {
		t.Fatal(err)
	}
	publisher := &coveragePublishRecorder{}
	if err := PublishPendingEvents(context.Background(), store, publisher, "runner-1"); err != nil {
		t.Fatal(err)
	}
	if len(publisher.messages) != 1 || publisher.messages[0].Subject != queue.Subject("events", "runner-1") || publisher.messages[0].ID != "event-1" || string(publisher.messages[0].Data) != "raw" {
		t.Fatalf("published messages = %#v", publisher.messages)
	}
	if pending, err := store.PendingEvents(10); err != nil || len(pending) != 0 {
		t.Fatalf("pending events = %#v, err=%v", pending, err)
	}

	if err := store.PutEvent(OutboxEvent{EventID: "event-2", OrderID: "order-1", Channel: "state", Sequence: 2, Envelope: "raw"}); err != nil {
		t.Fatal(err)
	}
	publisher.err = errors.New("publisher unavailable")
	if err := PublishPendingEvents(context.Background(), store, publisher, "runner-1"); !errors.Is(err, publisher.err) {
		t.Fatalf("publisher error = %v", err)
	}

	closed, err := OpenStore(t.TempDir() + "/closed.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := PublishPendingEvents(context.Background(), closed, &coveragePublishRecorder{}, "runner-1"); err == nil {
		t.Fatal("closed store accepted")
	}

	markFailure, err := OpenStore(t.TempDir() + "/mark-failure.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := markFailure.PutEvent(OutboxEvent{EventID: "event-3", OrderID: "order-1", Channel: "state", Sequence: 1, Envelope: "raw"}); err != nil {
		t.Fatal(err)
	}
	closer := &coveragePublishRecorder{after: func() { _ = markFailure.db.Close() }}
	if err := PublishPendingEvents(context.Background(), markFailure, closer, "runner-1"); err == nil {
		t.Fatal("closed store marked event as published")
	}
}

func TestOrderRuntimeRejectsUnusableOrdersBeforeExecution(t *testing.T) {
	if err := (OrderRuntime{}).Handle(context.Background(), queue.Message{}); err == nil {
		t.Fatal("unconfigured runtime accepted an order")
	}

	store, err := OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	runtime := OrderRuntime{Store: store, Publisher: &runtimePublisher{}, StartClaimer: testStartClaimer{}, RunnerID: "runner-1", ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey}
	if err := runtime.Handle(context.Background(), queue.Message{Data: []byte("bad envelope")}); err == nil {
		t.Fatal("malformed envelope accepted")
	}

	otherKey, err := protocol.GenerateSigningKey("other", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	order := coverageOrder("order-1", "run-1", "runner-1", time.Now().UTC())
	if err := runtime.Handle(context.Background(), queue.Message{Data: signedCoverageOrder(t, otherKey, order)}); err == nil {
		t.Fatal("order signed by another key accepted")
	}
	if err := runtime.Handle(context.Background(), queue.Message{Data: signedCoveragePayload(t, controlKey, []byte("bad payload"))}); err == nil {
		t.Fatal("malformed order payload accepted")
	}

	order.SecretRefs = map[string]string{"TOKEN": "secret"}
	if err := runtime.Handle(context.Background(), queue.Message{Data: signedCoverageOrder(t, controlKey, order)}); !errors.Is(err, ErrSecretDeliveryUnavailable) {
		t.Fatalf("missing secret fetcher error = %v", err)
	}
	order.SecretRefs = nil
	runtime.StartClaimer = testStartClaimer{err: errors.New("claim unavailable")}
	if err := runtime.Handle(context.Background(), queue.Message{Data: signedCoverageOrder(t, controlKey, order)}); err == nil {
		t.Fatal("start claim failure was ignored")
	}
}

func TestOrderRuntimeValidatesCancellationOrders(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	runtime := OrderRuntime{Store: store, Publisher: &runtimePublisher{}, StartClaimer: testStartClaimer{}, RunnerID: "runner-1", ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey}
	now := time.Now().UTC()
	base := coverageOrder("cancel-1", "run-1", "runner-1", now)
	base.Type = protocol.OrderCancel
	base.TargetOrderID = "target-1"
	for _, test := range []struct {
		name  string
		order protocol.OrderPayload
	}{
		{name: "expired", order: func() protocol.OrderPayload {
			order := base
			order.IssuedAt = now.Add(-2 * time.Minute)
			order.NotBefore = now.Add(-2 * time.Minute)
			order.ExpiresAt = now.Add(-time.Minute)
			return order
		}()},
		{name: "wrong runner", order: func() protocol.OrderPayload { order := base; order.RunnerID = "other"; return order }()},
		{name: "missing execution fields", order: func() protocol.OrderPayload { order := base; order.Command = nil; return order }()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := runtime.Handle(context.Background(), queue.Message{Data: signedCoverageOrder(t, controlKey, test.order)}); err == nil {
				t.Fatal("invalid cancellation accepted")
			}
		})
	}
}

func TestApplyRunnerControlRejectsBadPayloads(t *testing.T) {
	key, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := protocol.GenerateSigningKey("other", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(t *testing.T, key protocol.SigningKey, payload []byte) []byte {
		t.Helper()
		envelope, err := key.SignEvent(payload)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := protocol.EncodeEnvelope(envelope)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	validSubject := queue.Subject("control", "runner-1")
	var capacity atomic.Int64
	if err := ApplyRunnerControl(context.Background(), queue.Message{Subject: validSubject, Data: sign(t, otherKey, []byte("payload"))}, "runner-1", key.Public.PublicKey, &capacity); err == nil {
		t.Fatal("control signed by another key accepted")
	}
	if err := ApplyRunnerControl(context.Background(), queue.Message{Subject: validSubject, Data: sign(t, key, []byte("payload"))}, "runner-1", key.Public.PublicKey, &capacity); err == nil {
		t.Fatal("malformed control payload accepted")
	}
	payload, err := protocol.EncodeRunnerControlPayload(protocol.RunnerControlPayload{Version: protocol.ProtocolVersion, Type: protocol.RunnerControlCapacity, RunnerID: "other", Capacity: 1, IssuedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyRunnerControl(context.Background(), queue.Message{Subject: validSubject, Data: sign(t, key, payload)}, "runner-1", key.Public.PublicKey, &capacity); err == nil {
		t.Fatal("control for another runner accepted")
	}
}

func TestOrderRuntimeEventPersistenceReturnsErrors(t *testing.T) {
	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	order := coverageOrder("event-order", "event-run", "runner-1", time.Now().UTC())
	closed, err := OpenStore(t.TempDir() + "/closed.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	runtime := OrderRuntime{Store: closed, SigningKey: key}
	if err := runtime.publishEvent(context.Background(), order, eventRecord{typeName: protocol.EventAccepted, sequence: 1, channel: "state"}); err == nil {
		t.Fatal("closed store persisted event")
	}

	store, err := OpenStore(t.TempDir() + "/invalid-key.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runtime.Store = store
	runtime.SigningKey = protocol.SigningKey{}
	if err := runtime.persistEvent(order, eventRecord{typeName: protocol.EventAccepted, sequence: 1, channel: "state"}); err == nil {
		t.Fatal("invalid signing key persisted event")
	}
}

func TestNATSSecretFetcherRejectsInvalidResponses(t *testing.T) {
	now := time.Now().UTC()
	runnerKey, err := protocol.GenerateSigningKey("runner", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := protocol.GenerateSigningKey("control", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.SecretDeliveryRequest{Version: protocol.ProtocolVersion, RequestID: "request", OrderID: "order", RunID: "run", Attempt: 1, LeaseToken: "lease", RunnerID: "runner", RunnerSessionID: "session", FencingToken: 1, ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret"}, IssuedAt: now}

	validResponse := func(reply protocol.SecretDeliveryResponse) []byte {
		t.Helper()
		payload, err := protocol.EncodeSecretDeliveryResponse(reply)
		if err != nil {
			t.Fatal(err)
		}
		envelope := protocol.NewEnvelope(controlKey.ID, payload)
		if err := envelope.Sign(controlKey.Private, protocol.SecretDeliverySignatureDomain); err != nil {
			t.Fatal(err)
		}
		raw, err := protocol.EncodeEnvelope(envelope)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	cases := []struct {
		name string
		raw  []byte
	}{
		{name: "invalid envelope", raw: []byte("bad")},
		{name: "invalid payload", raw: signedCoveragePayload(t, controlKey, []byte("bad response"))},
		{name: "request mismatch", raw: validResponse(protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion, RequestID: "other", RespondedAt: now, Values: map[string]string{"TOKEN": "value"}})},
		{name: "error reply", raw: validResponse(protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion, RequestID: request.RequestID, RespondedAt: now, Error: "denied"})},
		{name: "incomplete reply", raw: validResponse(protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion, RequestID: request.RequestID, RespondedAt: now})},
		{name: "unexpected value", raw: validResponse(protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion, RequestID: request.RequestID, RespondedAt: now, Values: map[string]string{"OTHER": "value"}})},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fetcher := NewNATSSecretFetcher(coverageRequester{response: queue.Message{Data: test.raw}}, runnerKey, controlKey.Public.PublicKey)
			if _, err := fetcher.Fetch(context.Background(), request); !errors.Is(err, ErrSecretDeliveryUnavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDurableRecoveryRejectsClosedStores(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	key, err := protocol.GenerateSigningKey("runner", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverDurable(store, "old"); err == nil {
		t.Fatal("closed durable store recovered orders")
	}
	if _, err := RecoverDurableSigned(store, "old", key); err == nil {
		t.Fatal("closed signed durable store recovered orders")
	}
	if _, err := RecoverDurable(nil, "old"); err == nil {
		t.Fatal("nil durable store accepted")
	}
	if _, err := RecoverDurableSigned(nil, "old", key); err == nil {
		t.Fatal("nil signed durable store accepted")
	}
}

func TestLocalStoreReturnsErrorsAfterClose(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	key, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	order := coverageOrder("order-closed", "run-closed", "runner-1", time.Now().UTC())
	raw := signedCoverageOrder(t, key, order)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSigningKey(key); err == nil {
		t.Fatal("closed store saved signing key")
	}
	if _, _, err := store.LoadSigningKey(); err == nil {
		t.Fatal("closed store loaded signing key")
	}
	if err := store.SaveConnection(RunnerConnection{RunnerID: "runner-1"}); err == nil {
		t.Fatal("closed store saved connection")
	}
	if _, _, err := store.LoadConnection(); err == nil {
		t.Fatal("closed store loaded connection")
	}
	if err := store.PutOrder(InboxOrder{OrderID: "order", ExecutionAttemptID: "attempt", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", Envelope: "raw"}); err == nil {
		t.Fatal("closed store saved order")
	}
	if err := store.ClaimOrder("order", "boot", 1); err == nil {
		t.Fatal("closed store claimed order")
	}
	if err := store.MarkProcessStarted("order"); err == nil {
		t.Fatal("closed store marked process started")
	}
	if err := store.FinishOrder("order", "FAILED", "closed"); err == nil {
		t.Fatal("closed store finished order")
	}
	if err := store.PutEvent(OutboxEvent{EventID: "event", OrderID: "order", Channel: "state", Sequence: 1, Envelope: "raw"}); err == nil {
		t.Fatal("closed store saved event")
	}
	if _, err := store.PendingEvents(1); err == nil {
		t.Fatal("closed store listed events")
	}
	if err := store.MarkEventPublished("event"); err == nil {
		t.Fatal("closed store marked event")
	}
	if _, err := store.CompactPublishedEvents(time.Hour, 1); err == nil {
		t.Fatal("closed store compacted events")
	}
	if err := store.Put("key", map[string]string{"value": "x"}); err == nil {
		t.Fatal("closed store saved metadata")
	}
	if _, err := store.Get("key"); err == nil {
		t.Fatal("closed store loaded metadata")
	}
	if _, err := store.AcceptOrder(raw, protocol.Keyring{"control-plane": {ID: key.ID, PublicKey: key.Public.PublicKey}}, time.Now().UTC(), order.RunnerID, order.RunID, order.Attempt, order.LeaseToken, time.Second); err == nil {
		t.Fatal("closed store accepted order")
	}
}

func coverageOrder(orderID, runID, runnerID string, now time.Time) protocol.OrderPayload {
	return protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: orderID, RunID: runID, TaskID: "task-1", Attempt: 1, LeaseToken: "lease-1", RunnerID: runnerID, RunnerSessionID: "session-1", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"true"}, WorkingDir: ".", DurationSeconds: 1}
}

func signedCoverageOrder(t *testing.T, key protocol.SigningKey, order protocol.OrderPayload) []byte {
	t.Helper()
	payload, err := protocol.EncodeOrderPayload(order)
	if err != nil {
		t.Fatal(err)
	}
	return signedCoveragePayload(t, key, payload)
}

func signedCoveragePayload(t *testing.T, key protocol.SigningKey, payload []byte) []byte {
	t.Helper()
	envelope, err := key.SignOrder(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
