package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

type OrderType string
type EventType string

type ResourceLimits struct {
	MaxOutputBytes uint64 `json:"max_output_bytes"`
	MaxMemoryBytes uint64 `json:"max_memory_bytes"`
	MaxProcesses   uint64 `json:"max_processes"`
}

type OrderPayload struct {
	Version        uint8             `json:"version"`
	OrderID        string            `json:"order_id"`
	RunID          string            `json:"run_id"`
	TaskID         string            `json:"task_id"`
	Attempt        uint32            `json:"attempt"`
	LeaseToken     string            `json:"lease_token"`
	RunnerID       string            `json:"runner_id"`
	IssuedAt       time.Time         `json:"issued_at"`
	NotBefore      time.Time         `json:"not_before"`
	ExpiresAt      time.Time         `json:"expires_at"`
	Type           OrderType         `json:"type"`
	Command        []string          `json:"command"`
	WorkingDir     string            `json:"working_dir"`
	SecretRefs     []string          `json:"secret_refs,omitempty"`
	TimeoutSeconds uint32            `json:"timeout_seconds"`
	Limits         ResourceLimits    `json:"limits"`
	Resources      map[string]string `json:"resources,omitempty"`
}

type EventPayload struct {
	Version      uint8            `json:"version"`
	EventID      string           `json:"event_id"`
	OrderID      string           `json:"order_id"`
	RunID        string           `json:"run_id"`
	TaskID       string           `json:"task_id"`
	Attempt      uint32           `json:"attempt"`
	LeaseToken   string           `json:"lease_token"`
	RunnerID     string           `json:"runner_id"`
	Sequence     uint64           `json:"sequence"`
	ObservedAt   time.Time        `json:"observed_at"`
	Type         EventType        `json:"type"`
	Result       string           `json:"result,omitempty"`
	Metrics      map[string]int64 `json:"metrics,omitempty"`
	OutputDigest string           `json:"output_digest,omitempty"`
	Error        string           `json:"error,omitempty"`
}

func DecodeOrderPayload(raw []byte) (OrderPayload, error) {
	framed, err := decodePayloadFrame(raw)
	if err != nil {
		return OrderPayload{}, err
	}
	var payload OrderPayload
	if err := json.Unmarshal(framed, &payload); err != nil {
		return OrderPayload{}, err
	}
	if payload.Version != ProtocolVersion {
		return OrderPayload{}, errors.New("unsupported order payload version")
	}
	if !payload.Type.Valid() {
		return OrderPayload{}, errors.New("unsupported order type")
	}
	return payload, nil
}

func DecodeEventPayload(raw []byte) (EventPayload, error) {
	framed, err := decodePayloadFrame(raw)
	if err != nil {
		return EventPayload{}, err
	}
	var payload EventPayload
	if err := json.Unmarshal(framed, &payload); err != nil {
		return EventPayload{}, err
	}
	if payload.Version != ProtocolVersion {
		return EventPayload{}, errors.New("unsupported event payload version")
	}
	if !payload.Type.Valid() {
		return EventPayload{}, errors.New("unsupported event type")
	}
	return payload, nil
}
