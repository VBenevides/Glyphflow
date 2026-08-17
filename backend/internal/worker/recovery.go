package worker

import (
	"crypto/ed25519"
	"errors"
	"sync"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

type OrderRecovery struct {
	mu     sync.Mutex
	bootID string
	orders map[string]string
}

func NewOrderRecovery(bootID string) *OrderRecovery {
	return &OrderRecovery{bootID: bootID, orders: make(map[string]string)}
}

func (r *OrderRecovery) Claim(orderID string) error {
	r.mu.Lock()
	r.orders[orderID] = r.bootID
	r.mu.Unlock()
	return nil
}

func (r *OrderRecovery) Recover(previousBootID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var unknown []string
	for orderID, bootID := range r.orders {
		if bootID == previousBootID {
			unknown = append(unknown, orderID)
		}
	}
	return unknown
}

func RecoverDurable(store *LocalStore, previousBootID string) ([]string, error) {
	if store == nil || previousBootID == "" {
		return nil, errors.New("durable recovery inputs are required")
	}
	return store.RecoverOrders(previousBootID)
}

func RecoverDurableSigned(store *LocalStore, previousBootID string, key protocol.SigningKey) ([]string, error) {
	if store == nil || previousBootID == "" || len(key.Private) != ed25519.PrivateKeySize {
		return nil, errors.New("signed durable recovery inputs are required")
	}
	return store.RecoverOrdersSigned(previousBootID, key)
}
