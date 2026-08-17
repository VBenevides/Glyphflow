package platform

import (
	"errors"
	"os"
	"path/filepath"
)

type ObjectLogStore struct {
	dir string
	max int
}

func NewObjectLogStore(dir string, maxBytes int) (*ObjectLogStore, error) {
	if dir == "" || maxBytes <= 0 {
		return nil, errors.New("object log store settings are required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &ObjectLogStore{dir: dir, max: maxBytes}, nil
}
func (s *ObjectLogStore) Put(id string, data []byte) error {
	if id == "" || len(data) > s.max {
		return errors.New("log object exceeds configured bound")
	}
	return os.WriteFile(filepath.Join(s.dir, id), data, 0600)
}
func (s *ObjectLogStore) Get(id string) ([]byte, error) { return os.ReadFile(filepath.Join(s.dir, id)) }
