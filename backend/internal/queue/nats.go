package queue

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type JetStream struct {
	conn   *nats.Conn
	stream jetstream.Stream
}

func ConnectJetStream(url string) (*JetStream, error) {
	conn, err := nats.Connect(url)
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
