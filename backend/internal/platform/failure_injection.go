package platform

import (
	"errors"
	"sync"
)

type FailureInjector struct {
	mu          sync.Mutex
	failures    map[string]int
	occurrences map[string]bool
}

func NewFailureInjector() *FailureInjector {
	return &FailureInjector{failures: map[string]int{}, occurrences: map[string]bool{}}
}
func (f *FailureInjector) FailNext(boundary string) {
	f.mu.Lock()
	f.failures[boundary]++
	f.mu.Unlock()
}
func (f *FailureInjector) RunOccurrence(id, boundary string, commit func() error) error {
	f.mu.Lock()
	if f.occurrences[id] {
		f.mu.Unlock()
		return nil
	}
	if f.failures[boundary] > 0 {
		f.failures[boundary]--
		f.mu.Unlock()
		return errors.New("injected failure at " + boundary)
	}
	f.mu.Unlock()
	if err := commit(); err != nil {
		return err
	}
	f.mu.Lock()
	f.occurrences[id] = true
	f.mu.Unlock()
	return nil
}
