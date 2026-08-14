package queue

import (
	"context"
	"errors"
	"fmt"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type JetStream struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	stream jetstream.Stream
}

type TLSConfig struct {
	CertificateFile string
	KeyFile         string
	CAFile          string
}

func (c TLSConfig) options() ([]nats.Option, error) {
	if c.CertificateFile == "" || c.KeyFile == "" || c.CAFile == "" {
		return nil, errors.New("NATS mutual TLS requires certificate, key, and CA files")
	}
	return []nats.Option{nats.ClientCert(c.CertificateFile, c.KeyFile), nats.RootCAs(c.CAFile), nats.Secure()}, nil
}

func ConnectJetStream(url string) (*JetStream, error) {
	parsed, err := urlpkg.Parse(url)
	if err != nil || parsed.Scheme != "tls" {
		return nil, errors.New("NATS mutual TLS is required")
	}
	return connectJetStream(url)
}

func ConnectJetStreamPlain(url string) (*JetStream, error) {
	parsed, err := urlpkg.Parse(url)
	if err != nil || parsed.Scheme != "nats" {
		return nil, errors.New("plain NATS requires a nats:// URL")
	}
	return connectJetStream(url)
}

func ConnectJetStreamTLS(url string, tls TLSConfig) (*JetStream, error) {
	options, err := tls.options()
	if err != nil {
		return nil, err
	}
	return connectJetStream(url, options...)
}

func connectJetStream(url string, options ...nats.Option) (*JetStream, error) {
	conn, err := nats.Connect(url, options...)
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{Name: "GLYPHFLOW", Subjects: []string{"glyphflow.orders.>", "glyphflow.events.>", "glyphflow.deadletter.>"}, Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy, MaxMsgSize: 1 << 20})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &JetStream{conn: conn, js: js, stream: stream}, nil
}

func (j *JetStream) Close() {
	if j != nil && j.conn != nil {
		j.conn.Drain()
	}
}
func (j *JetStream) StreamName() string {
	if j == nil {
		return ""
	}
	return j.stream.CachedInfo().Config.Name
}

func (j *JetStream) Publish(ctx context.Context, message Message) error {
	if j == nil || j.stream == nil || message.Subject == "" || len(message.Data) == 0 {
		return errors.New("queue and message data are required")
	}
	options := []jetstream.PublishOpt{}
	if message.ID != "" {
		options = append(options, jetstream.WithMsgID(message.ID))
	}
	_, err := j.js.Publish(ctx, message.Subject, message.Data, options...)
	return err
}

func (j *JetStream) Consumer(ctx context.Context, durable, subject string, maxPending int) (jetstream.Consumer, error) {
	if j == nil || j.stream == nil || durable == "" || subject == "" || maxPending < 1 {
		return nil, errors.New("consumer configuration is invalid")
	}
	return j.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: durable, FilterSubject: subject, AckPolicy: jetstream.AckExplicitPolicy,
		MaxDeliver: 5, MaxAckPending: maxPending, AckWait: 30 * time.Second,
	})
}

type Handler func(context.Context, Message) error

func (j *JetStream) ConsumeOne(ctx context.Context, consumer jetstream.Consumer, handler Handler) error {
	if consumer == nil || handler == nil {
		return errors.New("consumer and handler are required")
	}
	message, err := consumer.Next(jetstream.FetchMaxWait(30 * time.Second))
	if err != nil {
		return err
	}
	queueMessage := Message{Subject: message.Subject(), Data: message.Data(), ID: message.Headers().Get("Nats-Msg-Id")}
	if err := handler(ctx, queueMessage); err == nil {
		return message.DoubleAck(ctx)
	} else {
		metadata, metadataErr := message.Metadata()
		if metadataErr == nil && metadata.NumDelivered >= 5 {
			deadLetter := []byte(strings.TrimSpace(err.Error()))
			if len(deadLetter) > 4096 {
				deadLetter = deadLetter[:4096]
			}
			if publishErr := j.Publish(ctx, Message{Subject: "glyphflow.deadletter." + message.Subject(), Data: deadLetter}); publishErr != nil {
				return publishErr
			}
			return message.Ack()
		}
		return message.Nak()
	}
}

func Subject(kind, runnerID string) string { return fmt.Sprintf("glyphflow.%s.%s", kind, runnerID) }

type SubjectPermissions struct{ Allow []string }

type WorkerPermissionsConfig struct {
	Publish   SubjectPermissions
	Subscribe SubjectPermissions
}

func WorkerPermissions(runnerID string) WorkerPermissionsConfig {
	return WorkerPermissionsConfig{
		Publish:   SubjectPermissions{Allow: []string{Subject("events", runnerID)}},
		Subscribe: SubjectPermissions{Allow: []string{Subject("orders", runnerID)}},
	}
}

func AllowedWorkerSubject(subject, runnerID string) bool {
	return subject == Subject("orders", runnerID) || subject == Subject("events", runnerID)
}
