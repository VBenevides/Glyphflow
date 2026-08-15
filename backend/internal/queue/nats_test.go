package queue

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type testMessages struct {
	items   []jetstream.Msg
	stopped chan struct{}
	once    chan struct{}
}

func newTestMessages(items ...jetstream.Msg) *testMessages {
	return &testMessages{items: items, stopped: make(chan struct{}), once: make(chan struct{}, 1)}
}

func (m *testMessages) Next() (jetstream.Msg, error) {
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
}

func TestMutualTLSAndWorkerPermissions(t *testing.T) {
	if _, err := (TLSConfig{}).options(); err == nil {
		t.Fatal("incomplete TLS configuration was accepted")
	}
	permissions := WorkerPermissions("worker-1")
	if len(permissions.Publish.Allow) != 1 || permissions.Publish.Allow[0] != "glyphflow.events.worker-1" {
		t.Fatalf("unexpected publish permissions: %#v", permissions.Publish.Allow)
	}
	if len(permissions.Subscribe.Allow) != 2 || permissions.Subscribe.Allow[0] != "glyphflow.orders.worker-1" || permissions.Subscribe.Allow[1] != "glyphflow.control.worker-1" {
		t.Fatalf("unexpected subscribe permissions: %#v", permissions.Subscribe.Allow)
	}
	if AllowedWorkerSubject("glyphflow.orders.worker-2", "worker-1") || !AllowedWorkerSubject("glyphflow.events.worker-1", "worker-1") || !AllowedWorkerSubject("glyphflow.control.worker-1", "worker-1") {
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
