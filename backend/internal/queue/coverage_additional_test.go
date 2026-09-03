package queue

import (
	"context"
	"errors"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go/jetstream"
)

func TestJetStreamValidationCoverage(t *testing.T) {
	if _, err := ConnectJetStream("nats://localhost:4222"); err == nil {
		t.Fatal("plain NATS accepted by TLS connector")
	}
	if _, err := ConnectJetStreamPlainWithContext(context.Background(), "tls://localhost:4222"); err == nil {
		t.Fatal("TLS NATS accepted by plain connector")
	}
	if _, err := ConnectJetStreamPlainWithContext(nil, "not a URL"); err == nil {
		t.Fatal("invalid NATS URL accepted")
	}
	if _, err := ConnectJetStreamTLS("tls://localhost:4222", TLSConfig{}); err == nil {
		t.Fatal("incomplete TLS config accepted")
	}
	var j *JetStream
	if j.StreamName() != "" || j.Publish(context.Background(), Message{}) == nil {
		t.Fatal("nil JetStream accepted an operation")
	}
	if _, err := j.Request(context.Background(), Message{}, time.Second); err == nil {
		t.Fatal("nil JetStream accepted a request")
	}
	if err := j.ServeRequests(context.Background(), "subject", func(context.Context, Message) Message { return Message{} }); err == nil {
		t.Fatal("nil request server accepted")
	}
	if _, err := j.Consumer(context.Background(), "durable", "subject", 1); err == nil {
		t.Fatal("nil consumer accepted")
	}
	if err := j.ConsumeOne(context.Background(), nil, nil); err == nil {
		t.Fatal("invalid consumer handler accepted")
	}
	if err := j.ConsumeSubject(context.Background(), "durable", "subject", 1, nil); err == nil {
		t.Fatal("nil subject handler accepted")
	}
	if err := j.ConsumeConcurrent(context.Background(), nil, nil); err == nil {
		t.Fatal("invalid concurrent consumer accepted")
	}
	if _, err := (TLSConfig{}).options(); err == nil {
		t.Fatal("incomplete TLS options accepted")
	}
}

type coverageMetadataFailureMessage struct{ exhaustedMessage }

func (coverageMetadataFailureMessage) Metadata() (*jetstream.MsgMetadata, error) {
	return nil, context.Canceled
}

func TestQueueMessageAndSubjectBranchesCoverage(t *testing.T) {
	message := &exhaustedMessage{subject: Subject("events", "runner-1"), data: []byte("event"), headers: map[string][]string{"Correlation-ID": {"fallback"}}}
	if err := (&JetStream{}).processMessage(context.Background(), message, func(context.Context, Message) error { return nil }); err != nil || !message.acked {
		t.Fatalf("successful message = %v acked=%v", err, message.acked)
	}
	message = &exhaustedMessage{subject: Subject("events", "runner-1"), data: []byte("event"), meta: &jetstream.MsgMetadata{NumDelivered: 1}}
	if err := (&JetStream{}).processMessage(context.Background(), message, func(context.Context, Message) error { return context.Canceled }); err != nil || !message.nacked {
		t.Fatalf("retryable message = %v nacked=%v", err, message.nacked)
	}
	failure := &coverageMetadataFailureMessage{exhaustedMessage: exhaustedMessage{subject: Subject("events", "runner-1"), data: []byte("event")}}
	if err := (&JetStream{}).processMessage(context.Background(), failure, func(context.Context, Message) error { return context.Canceled }); err != nil || !failure.nacked {
		t.Fatalf("metadata failure message = %v nacked=%v", err, failure.nacked)
	}
	if boundedError(nil) != "" || boundedError(errors.New("  message  ")) != "message" || !utf8.ValidString(boundedError(errors.New(string([]byte{0xff})))) {
		t.Fatal("error bounding failed")
	}
	for _, header := range []string{"X-Correlation-ID", "Correlation-ID", "Nats-Correlation-ID"} {
		if got := correlationID(map[string][]string{header: {"id"}}); got != "id" {
			t.Fatalf("correlation header %s = %q", header, got)
		}
	}
	if correlationID(nil) != "" || runnerIDFromSubject("bad") != "" || runnerIDFromSubject("glyphflow.unknown.runner") != "" {
		t.Fatal("invalid subject metadata accepted")
	}
}
