package platform

import (
	"errors"
	"sync"
)

type RefreshFamily struct {
	mu      sync.Mutex
	active  map[string]string
	revoked map[string]bool
}

func NewRefreshFamily() *RefreshFamily {
	return &RefreshFamily{active: map[string]string{}, revoked: map[string]bool{}}
}
func (f *RefreshFamily) Issue(family, token string) {
	f.mu.Lock()
	f.active[family] = token
	f.mu.Unlock()
}
func (f *RefreshFamily) Rotate(family, old, next string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.revoked[family] || f.active[family] != old {
		f.revoked[family] = true
		return errors.New("refresh family replay")
	}
	f.active[family] = next
	return nil
}
