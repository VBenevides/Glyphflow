package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var _ EventStream = (*JetStream)(nil)
var _ RequestServer = (*JetStream)(nil)

type testMessages struct {
	items   []jetstream.Msg
	stopped chan struct{}
	once    chan struct{}
}

func newTestMessages(items ...jetstream.Msg) *testMessages {
	return &testMessages{items: items, stopped: make(chan struct{}), once: make(chan struct{}, 1)}
}

func (m *testMessages) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	if len(m.items) > 0 {
		message := m.items[0]
		m.items = m.items[1:]
		return message, nil
	}
	<-m.stopped
	return nil, jetstream.ErrMsgIteratorClosed
}

func (m *testMessages) Stop() {
	select {
	case m.once <- struct{}{}:
		close(m.stopped)
	default:
	}
}

func (m *testMessages) Drain() { m.Stop() }

type testConsumer struct {
	jetstream.Consumer
	messages jetstream.MessagesContext
}

func (c testConsumer) Messages(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	return c.messages, nil
}

type testMessage struct {
	jetstream.Msg
	subject string
}

func (m testMessage) Subject() string                 { return m.subject }
func (m testMessage) Data() []byte                    { return []byte("order") }
func (m testMessage) Headers() nats.Header            { return nats.Header{} }
func (m testMessage) DoubleAck(context.Context) error { return nil }

func TestQueueSubjects(t *testing.T) {
	if Subject("orders", "worker-1") != "glyphflow.orders.worker-1" {
		t.Fatal("unexpected order subject")
	}
	if Subject("events", "worker-1") != "glyphflow.events.worker-1" {
		t.Fatal("unexpected event subject")
	}
	if Subject("heartbeats", "worker-1") != "glyphflow.heartbeats.worker-1" {
		t.Fatal("unexpected heartbeat subject")
	}
}

func TestMutualTLSAndWorkerPermissions(t *testing.T) {
	if _, err := (TLSConfig{}).options(); err == nil {
		t.Fatal("incomplete TLS configuration was accepted")
	}
	permissions := WorkerPermissions("worker-1")
	if len(permissions.Publish.Allow) != 4 || permissions.Publish.Allow[0] != "glyphflow.events.worker-1" || permissions.Publish.Allow[1] != "glyphflow.heartbeats.worker-1" || permissions.Publish.Allow[2] != StartClaimSubject("worker-1") || permissions.Publish.Allow[3] != SecretDeliverySubject("worker-1") {
		t.Fatalf("unexpected publish permissions: %#v", permissions.Publish.Allow)
	}
	if len(permissions.Subscribe.Allow) != 2 || permissions.Subscribe.Allow[0] != "glyphflow.orders.worker-1" || permissions.Subscribe.Allow[1] != "glyphflow.control.worker-1" {
		t.Fatalf("unexpected subscribe permissions: %#v", permissions.Subscribe.Allow)
	}
	if AllowedWorkerSubject("glyphflow.orders.worker-2", "worker-1") || !AllowedWorkerSubject("glyphflow.events.worker-1", "worker-1") || !AllowedWorkerSubject("glyphflow.heartbeats.worker-1", "worker-1") || !AllowedWorkerSubject("glyphflow.control.worker-1", "worker-1") || !AllowedWorkerSubject(StartClaimSubject("worker-1"), "worker-1") {
		t.Fatal("worker subject isolation failed")
	}
}

func TestQueueDeliveryDefaults(t *testing.T) {
	if Subject("deadletter", "glyphflow.orders.worker-1") != "glyphflow.deadletter.glyphflow.orders.worker-1" {
		t.Fatal("unexpected dead-letter subject")
	}
}

func TestConnectJetStreamRequiresMutualTLS(t *testing.T) {
	if _, err := ConnectJetStream("nats://localhost:4222"); err == nil {
		t.Fatal("plaintext NATS connection was accepted")
	}
}

func TestConnectJetStreamWithContextStopsRetryingWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ConnectJetStreamPlainWithContext(ctx, "nats://127.0.0.1:4222"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled NATS connection = %v", err)
	}
}

func TestConsumeConcurrentRunsDeliveredMessagesTogether(t *testing.T) {
	messages := newTestMessages(testMessage{subject: "orders"}, testMessage{subject: "orders"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- (&JetStream{}).ConsumeConcurrent(ctx, testConsumer{messages: messages}, func(context.Context, Message) error {
			started <- struct{}{}
			<-release
			return nil
		})
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("delivered messages were not handled concurrently")
		}
	}
	close(release)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConsumeConcurrentCancelsBlockedHandler(t *testing.T) {
	messages := newTestMessages(testMessage{subject: "orders"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		done <- (&JetStream{}).ConsumeConcurrent(ctx, testConsumer{messages: messages}, func(handlerCtx context.Context, _ Message) error {
			close(started)
			<-handlerCtx.Done()
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("message handler did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop after cancellation")
	}
}

type exhaustedMessage struct {
	subject string
	data    []byte
	headers nats.Header
	meta    *jetstream.MsgMetadata
	acked   bool
	nacked  bool
}

func (m *exhaustedMessage) Metadata() (*jetstream.MsgMetadata, error) { return m.meta, nil }
func (m *exhaustedMessage) Data() []byte                              { return m.data }
func (m *exhaustedMessage) Headers() nats.Header                      { return m.headers }
func (m *exhaustedMessage) Subject() string                           { return m.subject }
func (m *exhaustedMessage) Reply() string                             { return "" }
func (m *exhaustedMessage) Ack() error                                { m.acked = true; return nil }
func (m *exhaustedMessage) DoubleAck(context.Context) error           { m.acked = true; return nil }
func (m *exhaustedMessage) Nak() error                                { m.nacked = true; return nil }
func (*exhaustedMessage) NakWithDelay(time.Duration) error            { return nil }
func (*exhaustedMessage) InProgress() error                           { return nil }
func (*exhaustedMessage) Term() error                                 { return nil }
func (*exhaustedMessage) TermWithReason(string) error                 { return nil }

func TestDeadLetterPersistenceFailureNacksWithoutAck(t *testing.T) {
	message := &exhaustedMessage{
		subject: Subject("events", "runner-1"), data: []byte("signed-payload"),
		headers: nats.Header{"Nats-Msg-Id": []string{"event-1"}, "X-Correlation-ID": []string{"corr-1"}},
		meta:    &jetstream.MsgMetadata{Stream: "GLYPHFLOW", Consumer: "control-plane", NumDelivered: 5, Timestamp: time.Now().Add(-time.Minute), Sequence: jetstream.SequencePair{Stream: 42}},
	}
	var record DeadLetter
	stream := &JetStream{}
	stream.SetDeadLetterSink(func(_ context.Context, got DeadLetter) error { record = got; return errors.New("database unavailable") })
	tooLong := strings.Repeat("diagnostic ", 1000)
	err := stream.processMessage(context.Background(), message, func(context.Context, Message) error { return errors.New(tooLong) })
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("expected persistence error, got %v", err)
	}
	if message.acked || !message.nacked {
		t.Fatal("message was not NACKed after dead-letter persistence failed")
	}
	if record.Stream != "GLYPHFLOW" || record.Consumer != "control-plane" || record.MessageID != "event-1" || record.RunnerID != "runner-1" || record.CorrelationID != "corr-1" {
		t.Fatalf("dead-letter identity was not preserved: %#v", record)
	}
	if string(record.Payload) != "signed-payload" || len(record.Error) != 4096 || record.Attempts != 5 {
		t.Fatalf("dead-letter bounds or diagnostics were not preserved: payload=%q error=%d attempts=%d", record.Payload, len(record.Error), record.Attempts)
	}
}

func TestDeadLetterPublicationFailureNacksWithoutAck(t *testing.T) {
	message := &exhaustedMessage{
		subject: Subject("events", "runner-1"), data: []byte("signed-payload"),
		meta: &jetstream.MsgMetadata{Stream: "GLYPHFLOW", Consumer: "control-plane", NumDelivered: 5, Timestamp: time.Now().UTC()},
	}
	stream := &JetStream{}
	err := stream.processMessage(context.Background(), message, func(context.Context, Message) error { return errors.New("unknown attempt") })
	if err == nil || !strings.Contains(err.Error(), "queue and message data are required") {
		t.Fatalf("expected dead-letter publication error, got %v", err)
	}
	if message.acked || !message.nacked {
		t.Fatal("message was not NACKed after dead-letter publication failed")
	}
}

func TestDeadLetterNoticeFailureNacksWithoutAck(t *testing.T) {
	message := &exhaustedMessage{
		subject: Subject("events", "runner-1"), data: []byte("signed-payload"),
		meta: &jetstream.MsgMetadata{Stream: "GLYPHFLOW", Consumer: "control-plane", NumDelivered: 5, Timestamp: time.Now().UTC()},
	}
	stream := &JetStream{}
	stream.SetDeadLetterSink(func(context.Context, DeadLetter) error { return nil })
	err := stream.processMessage(context.Background(), message, func(context.Context, Message) error { return errors.New("signature rejected") })
	if err == nil || !strings.Contains(err.Error(), "queue and message data are required") {
		t.Fatalf("expected dead-letter notice error, got %v", err)
	}
	if message.acked || !message.nacked {
		t.Fatal("message was not NACKed after dead-letter notice failed")
	}
}

func TestRunnerIDFromSubjectKeepsDottedIDs(t *testing.T) {
	if got := runnerIDFromSubject("glyphflow.events.runner.eu-west"); got != "runner.eu-west" {
		t.Fatalf("runner ID = %q", got)
	}
	if got := runnerIDFromSubject("glyphflow.heartbeats.runner.eu-west"); got != "runner.eu-west" {
		t.Fatalf("heartbeat runner ID = %q", got)
	}
	if got := runnerIDFromSubject("glyphflow.deadletter.glyphflow.events.runner-1"); got != "" {
		t.Fatalf("dead-letter subject was treated as a runner subject: %q", got)
	}
}
