package worker

import (
	"encoding/json"
	"os"
	"sync"
)

type LocalStore struct {
	path     string
	mu       sync.Mutex
	Messages map[string]json.RawMessage
}

func OpenStore(path string) (*LocalStore, error) {
	s := &LocalStore{path: path, Messages: map[string]json.RawMessage{}}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s.Messages); err != nil {
			return nil, err
		}
	}
	return s, nil
}
func (s *LocalStore) Put(id string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Messages[id]; ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.Messages[id] = raw
	return s.flush()
}
func (s *LocalStore) flush() error {
	raw, err := json.Marshal(s.Messages)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0600)
}
