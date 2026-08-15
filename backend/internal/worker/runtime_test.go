package worker

import (
	"context"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

type runtimePublisher struct{ messages []queue.Message }

func (p *runtimePublisher) Publish(_ context.Context, message queue.Message) error {
	p.messages = append(p.messages, message)
	return nil
}

func TestOrderRuntimeExecutesOrderAndPublishesEvents(t *testing.T) {
	local, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	now := time.Now().UTC()
	order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "attempt-1", RunID: "run-1", TaskID: "task-1", Attempt: 1, LeaseToken: "lease-1", RunnerID: "runner-1", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"printf", "ok"}, WorkingDir: directory, TimeoutSeconds: 1}
	rawPayload, err := protocol.EncodeOrderPayload(order)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := controlKey.SignOrder(rawPayload)
	if err != nil {
		t.Fatal(err)
	}
	rawOrder, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	publisher := &runtimePublisher{}
	runtime := OrderRuntime{Store: local, Publisher: publisher, RunnerID: order.RunnerID, ExecutorBootID: "boot-1", ProcessID: 1, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Executor: Executor{Roots: []string{directory}, MaxOutputBytes: 1024}}
	if err := runtime.Handle(context.Background(), queue.Message{Subject: queue.Subject("orders", order.RunnerID), ID: order.OrderID, Data: rawOrder}); err != nil {
		t.Fatal(err)
	}
	if len(publisher.messages) != 4 {
		t.Fatalf("published events = %d, want 4", len(publisher.messages))
	}
	var types []protocol.EventType
	var terminalExitCode *int
	for _, message := range publisher.messages {
		decoded, err := protocol.DecodeEnvelope(message.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := decoded.VerifyEvent(workerKey.Public.PublicKey); err != nil {
			t.Fatal(err)
		}
		raw, err := decoded.PayloadBytes()
		if err != nil {
			t.Fatal(err)
		}
		event, err := protocol.DecodeEventPayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		types = append(types, event.Type)
		if event.Type == protocol.EventCompleted {
			terminalExitCode = event.ExitCode
		}
	}
	if want := []protocol.EventType{protocol.EventAccepted, protocol.EventStarted, protocol.EventLogChunk, protocol.EventCompleted}; len(types) != len(want) {
		t.Fatalf("event types = %v", types)
	} else {
		for i := range want {
			if types[i] != want[i] {
				t.Fatalf("event types = %v, want %v", types, want)
			}
		}
	}
	if terminalExitCode == nil || *terminalExitCode != 0 {
		t.Fatalf("terminal exit code = %v, want 0", terminalExitCode)
	}
}
