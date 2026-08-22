package queue

import (
	"context"
	"errors"
	"fmt"
	urlpkg "net/url"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type JetStream struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	stream jetstream.Stream
}

const UnlimitedPending = -1

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
	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{Name: "GLYPHFLOW", Subjects: []string{"glyphflow.orders.>", "glyphflow.events.>", "glyphflow.control.>", "glyphflow.deadletter.>"}, Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy, MaxMsgSize: 1 << 20})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &JetStream{conn: conn, js: js, stream: stream}, nil
}

func (j *JetStream) Close() {
	if j != nil && j.conn != nil {
		j.conn.Close()
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

func (j *JetStream) Request(ctx context.Context, message Message, timeout time.Duration) (Message, error) {
	if j == nil || j.conn == nil || message.Subject == "" || len(message.Data) == 0 || timeout <= 0 {
		return Message{}, errors.New("request and timeout are required")
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := j.conn.RequestWithContext(requestCtx, message.Subject, message.Data)
	if err != nil {
		return Message{}, err
	}
	return Message{Subject: response.Subject, Data: response.Data}, nil
}

type RequestHandler func(context.Context, Message) Message

func (j *JetStream) ServeRequests(ctx context.Context, subject string, handler RequestHandler) error {
	if j == nil || j.conn == nil || subject == "" || handler == nil {
		return errors.New("request server is not configured")
	}
	subscription, err := j.conn.Subscribe(subject, func(message *nats.Msg) {
		response := handler(ctx, Message{Subject: message.Subject, Data: message.Data})
		if len(response.Data) > 0 {
			_ = message.Respond(response.Data)
		}
	})
	if err != nil {
		return err
	}
	defer subscription.Unsubscribe()
	if err := j.conn.Flush(); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func (j *JetStream) Consumer(ctx context.Context, durable, subject string, maxPending int) (jetstream.Consumer, error) {
	if j == nil || j.stream == nil || durable == "" || subject == "" || maxPending == 0 || maxPending < UnlimitedPending {
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
	message, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil
		}
		return err
	}
	return j.processMessage(ctx, message, handler)
}

func (j *JetStream) ConsumeSubject(ctx context.Context, durable, subject string, maxPending int, handler Handler) error {
	consumer, err := j.Consumer(ctx, durable, subject, maxPending)
	if err != nil {
		return err
	}
	return j.ConsumeOne(ctx, consumer, handler)
}

func (j *JetStream) ConsumeConcurrent(ctx context.Context, consumer jetstream.Consumer, handler Handler) error {
	if consumer == nil || handler == nil {
		return errors.New("consumer and handler are required")
	}
	messages, err := consumer.Messages(jetstream.PullMaxMessages(1))
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			messages.Stop()
		case <-stop:
		}
	}()

	var workers sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	setFirstErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	for ctx.Err() == nil {
		message, nextErr := messages.Next()
		if nextErr != nil {
			if ctx.Err() == nil && !errors.Is(nextErr, jetstream.ErrMsgIteratorClosed) {
				setFirstErr(nextErr)
			}
			break
		}
		workers.Add(1)
		go func(message jetstream.Msg) {
			defer workers.Done()
			if err := j.processMessage(ctx, message, handler); err != nil {
				setFirstErr(err)
				messages.Stop()
			}
		}(message)
	}
	messages.Stop()
	workers.Wait()
	errMu.Lock()
	defer errMu.Unlock()
	return firstErr
}

func (j *JetStream) processMessage(ctx context.Context, message jetstream.Msg, handler Handler) error {
	keepAlive := time.NewTicker(10 * time.Second)
	keepAliveStop := make(chan struct{})
	keepAliveExited := make(chan struct{})
	go func() {
		defer close(keepAliveExited)
		for {
			select {
			case <-keepAlive.C:
				_ = message.InProgress()
			case <-ctx.Done():
				return
			case <-keepAliveStop:
				return
			}
		}
	}()
	defer func() {
		keepAlive.Stop()
		close(keepAliveStop)
		<-keepAliveExited
	}()
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

func StartClaimSubject(runnerID string) string { return "glyphflow.start." + runnerID }

type SubjectPermissions struct{ Allow []string }

type WorkerPermissionsConfig struct {
	Publish   SubjectPermissions
	Subscribe SubjectPermissions
}

func WorkerPermissions(runnerID string) WorkerPermissionsConfig {
	return WorkerPermissionsConfig{
		Publish:   SubjectPermissions{Allow: []string{Subject("events", runnerID), StartClaimSubject(runnerID)}},
		Subscribe: SubjectPermissions{Allow: []string{Subject("orders", runnerID), Subject("control", runnerID)}},
	}
}

func AllowedWorkerSubject(subject, runnerID string) bool {
	return subject == Subject("orders", runnerID) || subject == Subject("events", runnerID) || subject == Subject("control", runnerID) || subject == StartClaimSubject(runnerID)
}
