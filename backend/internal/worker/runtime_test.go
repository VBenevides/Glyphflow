package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
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

type concurrentRuntimePublisher struct {
	mu       sync.Mutex
	messages []queue.Message
}

type testStartClaimer struct{ err error }

func (c testStartClaimer) ClaimStart(context.Context, protocol.StartClaimPayload) error { return c.err }

type testSecretFetcher struct {
	values  map[string]string
	called  bool
	request protocol.SecretDeliveryRequest
}

func (f *testSecretFetcher) Fetch(_ context.Context, request protocol.SecretDeliveryRequest) (map[string]string, error) {
	f.called = true
	f.request = request
	return f.values, nil
}

func (p *concurrentRuntimePublisher) Publish(_ context.Context, message queue.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, message)
	return nil
}

func TestOrderRuntimeExecutesOrderAndPublishesEvents(t *testing.T) { // NOSONAR: this comprehensive runtime scenario intentionally covers durable acceptance, execution, event signing, and terminal publication together.
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
	order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "attempt-1", RunID: "run-1", TaskID: "task-1", TaskName: "Example task", TaskVersion: 2, Attempt: 1, LeaseToken: "lease-1", RunnerID: "runner-1", RunnerSessionID: "session-1", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"printf", "ok"}, WorkingDir: directory, DurationSeconds: 1}
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
	var terminal bytes.Buffer
	runtime := OrderRuntime{Store: local, Publisher: publisher, StartClaimer: testStartClaimer{}, RunnerID: order.RunnerID, ExecutorBootID: "boot-1", ProcessID: 1, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Executor: Executor{Roots: []string{directory}, MaxOutputBytes: 1024}, Writer: &terminal}
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
	lines := strings.Split(strings.TrimSpace(terminal.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("terminal lines = %q, want start and finish", terminal.String())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "> ") || !strings.Contains(line, `Task "Example task" v2 - ID "task-1"`) || !strings.Contains(line, `Run "run-1"`) {
			t.Fatalf("terminal line = %q, want prefixed task and run context", line)
		}
	}
}

func TestOrderRuntimeInjectsSecretsOnlyForExecution(t *testing.T) {
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
	order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "attempt-secret", RunID: "run-secret", TaskID: "task-secret", Attempt: 1, LeaseToken: "lease-secret", RunnerID: "runner-1", RunnerSessionID: "session-secret", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"sh", "-c", "test -n \"$TOKEN\""}, WorkingDir: directory, DurationSeconds: 1, SecretRefs: map[string]string{"TOKEN": "secret-1"}}
	orderBytes, err := protocol.EncodeOrderPayload(order)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := controlKey.SignOrder(orderBytes)
	if err != nil {
		t.Fatal(err)
	}
	rawOrder, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &testSecretFetcher{values: map[string]string{"TOKEN": "runtime-secret"}}
	publisher := &runtimePublisher{}
	runtime := OrderRuntime{Store: local, Publisher: publisher, StartClaimer: testStartClaimer{}, SecretFetcher: fetcher, RunnerID: order.RunnerID, ExecutorBootID: "boot-secret", ProcessID: 1, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Executor: Executor{Roots: []string{directory}, MaxOutputBytes: 1024}}
	if err := runtime.Handle(context.Background(), queue.Message{Subject: queue.Subject("orders", order.RunnerID), ID: order.OrderID, Data: rawOrder}); err != nil {
		t.Fatal(err)
	}
	if !fetcher.called || fetcher.request.SecretRefs["TOKEN"] != "secret-1" {
		t.Fatalf("secret fetch request = %#v", fetcher.request)
	}
	var storedEnvelope string
	if err := local.db.QueryRow(`SELECT envelope FROM order_inbox WHERE order_id = ?`, order.OrderID).Scan(&storedEnvelope); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedEnvelope, "runtime-secret") {
		t.Fatal("durable order envelope contains the plaintext secret")
	}
	for _, message := range publisher.messages {
		if strings.Contains(string(message.Data), "runtime-secret") {
			t.Fatal("durable event contains the plaintext secret")
		}
	}
}

func TestOrderRuntimeVerifiesCancellationOrders(t *testing.T) {
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

	for _, test := range []struct {
		name      string
		unsigned  bool
		tampered  bool
		cancelled bool
	}{
		{name: "unsigned", unsigned: true},
		{name: "tampered", tampered: true},
		{name: "valid", cancelled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			active := &ActiveOrders{}
			item := active.put("attempt-1", func() {})
			now := time.Now().UTC()
			order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "cancel-1", RunID: "run-1", TaskID: "task-1", Attempt: 1, LeaseToken: "lease-1", RunnerID: "runner-1", RunnerSessionID: "session-1", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderCancel, Command: []string{"true"}, WorkingDir: ".", DurationSeconds: 1, TargetOrderID: "attempt-1"}
			rawPayload, err := protocol.EncodeOrderPayload(order)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := controlKey.SignOrder(rawPayload)
			if err != nil {
				t.Fatal(err)
			}
			if test.unsigned {
				envelope.Signature = ""
			}
			if test.tampered {
				tampered := append([]byte(nil), rawPayload...)
				tampered[0] ^= 1
				envelope.Payload = base64.StdEncoding.EncodeToString(tampered)
			}
			rawOrder, err := protocol.EncodeEnvelope(envelope)
			if err != nil {
				t.Fatal(err)
			}
			runtime := OrderRuntime{Store: local, Publisher: &runtimePublisher{}, StartClaimer: testStartClaimer{}, RunnerID: order.RunnerID, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Active: active}
			err = runtime.Handle(context.Background(), queue.Message{Subject: queue.Subject("orders", order.RunnerID), ID: order.OrderID, Data: rawOrder})
			if (err == nil) != test.cancelled {
				t.Fatalf("Handle() error = %v, cancelled = %v", err, test.cancelled)
			}
			if item.cancelled.Load() != test.cancelled {
				t.Fatalf("active cancellation = %v, want %v", item.cancelled.Load(), test.cancelled)
			}
		})
	}
}

func TestOrderRuntimeRunsOrdersConcurrently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell concurrency test requires POSIX tools")
	}
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
	started := []string{directory + "/started-1", directory + "/started-2"}
	finished := directory + "/finished"
	publisher := &concurrentRuntimePublisher{}
	runtime := OrderRuntime{Store: local, Publisher: publisher, StartClaimer: testStartClaimer{}, RunnerID: "runner-1", ExecutorBootID: "boot-1", ProcessID: 1, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Executor: Executor{Roots: []string{directory}, MaxOutputBytes: 1024}}
	orders := make([]queue.Message, 2)
	for index := range orders {
		now := time.Now().UTC()
		command := fmt.Sprintf("touch %q; while [ ! -f %q ] || [ ! -f %q ]; do sleep 0.01; done; printf x >> %q", started[index], started[0], started[1], finished)
		order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: fmt.Sprintf("attempt-%d", index+1), RunID: fmt.Sprintf("run-%d", index+1), TaskID: "task-1", Attempt: 1, LeaseToken: fmt.Sprintf("lease-%d", index+1), RunnerID: "runner-1", RunnerSessionID: fmt.Sprintf("session-%d", index+1), IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"sh", "-c", command}, WorkingDir: directory, DurationSeconds: 2}
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
		orders[index] = queue.Message{Subject: queue.Subject("orders", order.RunnerID), ID: order.OrderID, Data: rawOrder}
	}
	errs := make(chan error, len(orders))
	var waitGroup sync.WaitGroup
	for _, order := range orders {
		waitGroup.Add(1)
		go func(order queue.Message) {
			defer waitGroup.Done()
			errs <- runtime.Handle(context.Background(), order)
		}(order)
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(finished)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "xx" {
		t.Fatalf("finished markers = %q, want both concurrent orders to finish", content)
	}
}

func TestOrderRuntimeReportsTimeoutWithSystemCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell timeout test requires POSIX tools")
	}
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
	order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "attempt-timeout", RunID: "run-timeout", TaskID: "task-timeout", Attempt: 1, LeaseToken: "lease-timeout", RunnerID: "runner-1", RunnerSessionID: "session-timeout", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"sh", "-c", "sleep 2"}, WorkingDir: directory, DurationSeconds: 1}
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
	runtime := OrderRuntime{Store: local, Publisher: publisher, StartClaimer: testStartClaimer{}, RunnerID: order.RunnerID, ExecutorBootID: "boot-timeout", ProcessID: 1, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Executor: Executor{Roots: []string{directory}, MaxOutputBytes: 1024}}
	if err := runtime.Handle(context.Background(), queue.Message{Subject: queue.Subject("orders", order.RunnerID), ID: order.OrderID, Data: rawOrder}); err != nil {
		t.Fatal(err)
	}
	var timeoutEvent protocol.EventPayload
	for _, message := range publisher.messages {
		decoded, err := protocol.DecodeEnvelope(message.Data)
		if err != nil {
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
		if event.Type == protocol.EventTimedOut {
			timeoutEvent = event
		}
	}
	if timeoutEvent.Type != protocol.EventTimedOut || timeoutEvent.ExitCode == nil || *timeoutEvent.ExitCode != 6 || timeoutEvent.Error != "Timeout" {
		t.Fatalf("timeout event = %#v", timeoutEvent)
	}
}

func TestOrderRuntimeDiscardsRejectedStartClaim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell execution guard test requires POSIX tools")
	}
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
	marker := directory + "/started"
	now := time.Now().UTC()
	order := protocol.OrderPayload{Version: protocol.ProtocolVersion, OrderID: "attempt-rejected", RunID: "run-rejected", TaskID: "task-rejected", Attempt: 1, LeaseToken: "lease-rejected", RunnerID: "runner-1", RunnerSessionID: "session-rejected", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute, Command: []string{"touch", marker}, WorkingDir: directory, DurationSeconds: 1}
	orderBytes, err := protocol.EncodeOrderPayload(order)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := controlKey.SignOrder(orderBytes)
	if err != nil {
		t.Fatal(err)
	}
	rawOrder, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	publisher := &runtimePublisher{}
	runtime := OrderRuntime{Store: local, Publisher: publisher, StartClaimer: testStartClaimer{err: ErrStartRejected}, RunnerID: order.RunnerID, ExecutorBootID: "boot-rejected", ProcessID: 1, ControlPublicKey: controlKey.Public.PublicKey, SigningKey: workerKey, Executor: Executor{Roots: []string{directory}, MaxOutputBytes: 1024}}
	if err := runtime.Handle(context.Background(), queue.Message{Subject: queue.Subject("orders", order.RunnerID), ID: order.OrderID, Data: rawOrder}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker stat error = %v, worker executed rejected order", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published events = %d, want 0", len(publisher.messages))
	}
}
