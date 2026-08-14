package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type ExecutionSpec struct {
	TaskVersion string
	Command     []string
	WorkingDir  string
	Timeout     uint32
	SecretRefs  []string
	MaxOutput   uint64
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
