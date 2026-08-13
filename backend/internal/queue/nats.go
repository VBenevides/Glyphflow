package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type JetStream struct {
	conn   *nats.Conn
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
	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{Name: "GLYPHFLOW", Subjects: []string{"glyphflow.orders.>", "glyphflow.events.>"}, Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &JetStream{conn: conn, stream: stream}, nil
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
