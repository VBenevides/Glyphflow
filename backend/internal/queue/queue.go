package queue

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Message struct {
	Subject string
	Data    []byte
	ID      string
}
type Publisher interface {
	Publish(context.Context, Message) error
}
type Requester interface {
	Request(context.Context, Message, time.Duration) (Message, error)
}

type EventStream interface {
	Publisher
	ConsumeSubject(context.Context, string, string, int, Handler) error
}

type RequestServer interface {
	ServeRequests(context.Context, string, RequestHandler) error
}

type Consumer interface {
	Consume(context.Context) (Message, error)
}

type Memory struct {
	mu       sync.Mutex
	messages []Message
	wake     chan struct{}
}

func NewMemory() *Memory { return &Memory{wake: make(chan struct{}, 1)} }
func (m *Memory) Publish(_ context.Context, msg Message) error {
	if msg.Subject == "" || len(msg.Data) == 0 {
		return errors.New("subject and data are required")
	}
	m.mu.Lock()
	m.messages = append(m.messages, msg)
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}
func (m *Memory) Consume(ctx context.Context) (Message, error) {
	for {
		m.mu.Lock()
		if len(m.messages) > 0 {
			msg := m.messages[0]
			m.messages = m.messages[1:]
			m.mu.Unlock()
			return msg, nil
		}
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		case <-m.wake:
		case <-time.After(time.Second):
		}
	}
}
