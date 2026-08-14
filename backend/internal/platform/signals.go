package platform

import "sync"

type Signal struct {
	Kind, ID string
	Payload  map[string]string
}
type SignalBus struct {
	mu          sync.Mutex
	subscribers map[string][]chan Signal
}

func NewSignalBus() *SignalBus { return &SignalBus{subscribers: map[string][]chan Signal{}} }
func (b *SignalBus) Subscribe(kind string, buffer int) <-chan Signal {
	ch := make(chan Signal, buffer)
	b.mu.Lock()
	b.subscribers[kind] = append(b.subscribers[kind], ch)
	b.mu.Unlock()
	return ch
}
func (b *SignalBus) Publish(signal Signal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[signal.Kind] {
		select {
		case ch <- signal:
		default:
		}
	}
}
