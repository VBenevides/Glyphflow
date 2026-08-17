package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
)

type ExecutionSpec struct {
	TaskVersion string
	Command     []string
	WorkingDir  string
	Timeout     uint32
	SecretRefs  []string
	MaxOutput   uint64
}

type LogAccumulator struct {
	mu         sync.Mutex
	max, total int
	truncated  bool
	data       []byte
}

func NewLogAccumulator(max int) (*LogAccumulator, error) {
	if max <= 0 {
		return nil, errors.New("log output limit must be positive")
	}
	return &LogAccumulator{max: max}, nil
}
func (a *LogAccumulator) Append(chunk []byte) (accepted []byte, truncated bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	remaining := a.max - a.total
	if remaining <= 0 {
		a.truncated = true
		return nil, true
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
		a.truncated = true
		truncated = true
	}
	accepted = append([]byte(nil), chunk...)
	a.data = append(a.data, chunk...)
	a.total += len(chunk)
	return accepted, truncated
}
func (a *LogAccumulator) Bytes() ([]byte, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]byte(nil), a.data...), a.truncated
}

func ExecutionDigest(spec ExecutionSpec) (string, error) {
	if spec.TaskVersion == "" || len(spec.Command) == 0 || spec.WorkingDir == "" || spec.Timeout == 0 || spec.MaxOutput == 0 {
		return "", errors.New("execution specification is incomplete")
	}
	canonical, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func BoundLogChunk(raw []byte, max int) ([]byte, bool, error) {
	if max <= 0 {
		return nil, false, errors.New("log chunk limit must be positive")
	}
	if len(raw) <= max {
		return append([]byte(nil), raw...), false, nil
	}
	return append([]byte(nil), raw[:max]...), true, nil
}
