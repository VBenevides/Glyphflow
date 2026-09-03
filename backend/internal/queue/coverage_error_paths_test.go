package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type coverageJetStream struct {
	jetstream.JetStream
	err     error
	subject string
	payload []byte
}

func (j *coverageJetStream) Publish(_ context.Context, subject string, payload []byte, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	j.subject, j.payload = subject, payload
	return nil, j.err
}

type coverageStream struct {
	jetstream.Stream
	consumer jetstream.Consumer
	err      error
}

func (s coverageStream) CreateOrUpdateConsumer(context.Context, jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return s.consumer, s.err
}

func (s coverageStream) CachedInfo() *jetstream.StreamInfo {
	return &jetstream.StreamInfo{Config: jetstream.StreamConfig{Name: "GLYPHFLOW"}}
}

type coverageConsumer struct {
	jetstream.Consumer
	message  jetstream.Msg
	messages jetstream.MessagesContext
	err      error
}

func (c coverageConsumer) Next(...jetstream.FetchOpt) (jetstream.Msg, error) {
	return c.message, c.err
}

func (c coverageConsumer) Messages(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	return c.messages, c.err
}

type coverageMessages struct {
	err error
}

func (m coverageMessages) Next(...jetstream.NextOpt) (jetstream.Msg, error) { return nil, m.err }
func (coverageMessages) Stop()                                              {}
func (coverageMessages) Drain()                                             {}

type coverageAckMessage struct {
	exhaustedMessage
	doubleAckErr error
}

func (m *coverageAckMessage) DoubleAck(context.Context) error { return m.doubleAckErr }

func TestQueueConnectionAndTLSValidationEdges(t *testing.T) {
	if options, err := (TLSConfig{CertificateFile: "cert", KeyFile: "key", CAFile: "ca"}).options(); err != nil || len(options) != 3 {
		t.Fatalf("TLS options = %d, %v", len(options), err)
	}
	if _, err := ConnectJetStream("%"); err == nil {
		t.Fatal("malformed NATS URL was accepted")
	}
	if _, err := ConnectJetStreamTLS("tls://127.0.0.1:1", TLSConfig{CertificateFile: "cert", KeyFile: "key", CAFile: "ca"}); err == nil {
		t.Fatal("unreachable TLS NATS server was accepted")
	}
	if _, err := ConnectJetStreamTLSWithContext(context.Background(), "nats://127.0.0.1:4222", TLSConfig{}); err == nil {
		t.Fatal("incomplete TLS config was accepted")
	}
	if _, err := connectJetStream(nil, "nats://127.0.0.1:1", false); err == nil {
		t.Fatal("unreachable NATS server was accepted")
	}
	if got := (&JetStream{stream: coverageStream{}}).StreamName(); got != "GLYPHFLOW" {
		t.Fatalf("stream name = %q", got)
	}
}

func TestQueuePublishAndConsumerErrorPaths(t *testing.T) {
	ctx := context.Background()
	stream := coverageStream{}
	publishErr := errors.New("publish failed")
	js := &coverageJetStream{err: publishErr}
	j := &JetStream{js: js, stream: stream}
	if err := j.Publish(ctx, Message{Subject: "glyphflow.events.runner-1", Data: []byte("event"), ID: "event-1"}); !errors.Is(err, publishErr) {
		t.Fatalf("publish error = %v", err)
	}
	if js.subject != "glyphflow.events.runner-1" || string(js.payload) != "event" {
		t.Fatalf("published message = %q %q", js.subject, js.payload)
	}
	if _, err := j.Consumer(ctx, "", "subject", 1); err == nil {
		t.Fatal("empty durable was accepted")
	}
	if _, err := j.Consumer(ctx, "durable", "", 1); err == nil {
		t.Fatal("empty subject was accepted")
	}
	if _, err := j.Consumer(ctx, "durable", "subject", 0); err == nil {
		t.Fatal("zero pending limit was accepted")
	}
	consumerErr := errors.New("consumer failed")
	configured := &JetStream{stream: coverageStream{err: consumerErr}}
	if _, err := configured.Consumer(ctx, "durable", "subject", UnlimitedPending); !errors.Is(err, consumerErr) {
		t.Fatalf("consumer error = %v", err)
	}
	consumer := coverageConsumer{message: &exhaustedMessage{subject: "orders", data: []byte("order")}, err: consumerErr}
	if err := (&JetStream{}).ConsumeOne(ctx, consumer, func(context.Context, Message) error { return nil }); !errors.Is(err, consumerErr) {
		t.Fatalf("one-shot consumer error = %v", err)
	}
	if err := (&JetStream{}).ConsumeConcurrent(ctx, consumer, func(context.Context, Message) error { return nil }); !errors.Is(err, consumerErr) {
		t.Fatalf("concurrent consumer error = %v", err)
	}
	message := &exhaustedMessage{subject: "orders", data: []byte("order")}
	configured = &JetStream{stream: coverageStream{consumer: coverageConsumer{message: message}}}
	if err := configured.ConsumeSubject(ctx, "durable", "subject", UnlimitedPending, func(context.Context, Message) error { return nil }); err != nil || !message.acked {
		t.Fatalf("subject consume = %v acked=%v", err, message.acked)
	}
}

func TestQueueConsumeOneAndConcurrentErrorBranches(t *testing.T) {
	ctx := context.Background()
	timeoutConsumer := coverageConsumer{err: nats.ErrTimeout}
	if err := (&JetStream{}).ConsumeOne(ctx, timeoutConsumer, func(context.Context, Message) error { return nil }); err != nil {
		t.Fatalf("timeout consume = %v", err)
	}
	message := &exhaustedMessage{subject: "orders", data: []byte("order")}
	if err := (&JetStream{}).ConsumeOne(ctx, coverageConsumer{message: message}, func(context.Context, Message) error { return nil }); err != nil || !message.acked {
		t.Fatalf("successful consume = %v acked=%v", err, message.acked)
	}
	iteratorErr := errors.New("iterator failed")
	consumer := coverageConsumer{messages: coverageMessages{err: iteratorErr}}
	if err := (&JetStream{}).ConsumeConcurrent(ctx, consumer, func(context.Context, Message) error { return nil }); !errors.Is(err, iteratorErr) {
		t.Fatalf("iterator error = %v", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := (&JetStream{}).ConsumeConcurrent(ctx, coverageConsumer{messages: coverageMessages{err: iteratorErr}}, func(context.Context, Message) error { return nil }); err != nil {
		t.Fatalf("canceled concurrent consume = %v", err)
	}
}

func TestQueueDoubleAckErrorIsReturned(t *testing.T) {
	ackErr := errors.New("ack failed")
	message := &coverageAckMessage{exhaustedMessage: exhaustedMessage{subject: "orders", data: []byte("order")}, doubleAckErr: ackErr}
	if err := (&JetStream{}).processMessage(context.Background(), message, func(context.Context, Message) error { return nil }); !errors.Is(err, ackErr) {
		t.Fatalf("double ack error = %v", err)
	}
}

func TestQueueRequestAndServeValidationWithoutServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j := &JetStream{conn: &nats.Conn{}}
	if _, err := j.Request(ctx, Message{Subject: "subject", Data: []byte("request")}, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request = %v", err)
	}
	if err := j.ServeRequests(context.Background(), "invalid subject", func(context.Context, Message) Message { return Message{} }); err == nil {
		t.Fatal("invalid request subject was accepted")
	}
}

func TestMemoryQueueValidationAndCancellation(t *testing.T) {
	q := NewMemory()
	for _, message := range []Message{{}, {Subject: "subject"}, {Data: []byte("data")}} {
		if err := q.Publish(context.Background(), message); err == nil {
			t.Fatal("invalid memory message was accepted")
		}
	}
	if err := q.Publish(context.Background(), Message{Subject: "subject", Data: []byte("one")}); err != nil {
		t.Fatal(err)
	}
	if err := q.Publish(context.Background(), Message{Subject: "subject", Data: []byte("two")}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewMemory().Consume(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled memory consume = %v", err)
	}
}
