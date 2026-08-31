package worker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const bootstrapMagic = "GLYPHFLOW_RUNNER_BOOTSTRAP_V1"

type Bootstrap struct {
	Token                  string `json:"token"`
	RunnerID               string `json:"runner_id"`
	ControlPlaneURL        string `json:"control_plane_url"`
	ControlPublicKey       string `json:"control_public_key,omitempty"`
	NATSURL                string `json:"nats_url"`
	MaxMessageBytes        int    `json:"max_message_bytes"`
	AllowInsecureTransport bool   `json:"allow_insecure_transport,omitempty"`
	RunnerKeyID            string `json:"runner_key_id,omitempty"`
	RunnerPublicKey        string `json:"runner_public_key,omitempty"`
}

type RunnerConnection struct {
	RunnerID         string `json:"runner_id"`
	NATSURL          string `json:"nats_url"`
	MaxMessageBytes  int    `json:"max_message_bytes"`
	Capacity         int    `json:"capacity"`
	ControlPublicKey string `json:"control_public_key,omitempty"`
}

func PackBootstrap(executable []byte, bootstrap Bootstrap) ([]byte, error) {
	if bootstrap.Token == "" || bootstrap.RunnerID == "" || bootstrap.ControlPlaneURL == "" || bootstrap.MaxMessageBytes <= 0 {
		return nil, errors.New("runner bootstrap is incomplete")
	}
	publicKey, err := base64.RawStdEncoding.DecodeString(bootstrap.ControlPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("runner bootstrap control-plane public key is invalid")
	}
	payload, err := json.Marshal(bootstrap)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(executable)+len(payload)+8+len(bootstrapMagic))
	result = append(result, executable...)
	result = append(result, payload...)
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(payload)))
	result = append(result, length[:]...)
	result = append(result, bootstrapMagic...)
	return result, nil
}

func UnpackBootstrap(executable []byte) (*Bootstrap, error) {
	footerLength := len(bootstrapMagic) + 8
	if len(executable) < footerLength || !bytes.Equal(executable[len(executable)-len(bootstrapMagic):], []byte(bootstrapMagic)) {
		return nil, nil
	}
	lengthStart := len(executable) - len(bootstrapMagic) - 8
	payloadLength := binary.BigEndian.Uint64(executable[lengthStart:])
	if payloadLength > uint64(lengthStart) {
		return nil, errors.New("runner bootstrap payload is invalid")
	}
	payloadStart := lengthStart - int(payloadLength)
	var bootstrap Bootstrap
	if err := json.Unmarshal(executable[payloadStart:lengthStart], &bootstrap); err != nil {
		return nil, fmt.Errorf("runner bootstrap payload: %w", err)
	}
	return &bootstrap, nil
}

func LoadEmbeddedBootstrap() (*Bootstrap, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(executable)
	if err != nil {
		return nil, err
	}
	return UnpackBootstrap(raw)
}

func (b Bootstrap) Enroll(ctx context.Context) (RunnerConnection, error) {
	payload, err := json.Marshal(map[string]string{"runner_id": b.RunnerID, "token": b.Token, "key_id": b.RunnerKeyID, "public_key": b.RunnerPublicKey})
	if err != nil {
		return RunnerConnection{}, err
	}
	endpoint := strings.TrimRight(b.ControlPlaneURL, "/") + "/api/v1/runners/enroll"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return RunnerConnection{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return RunnerConnection{}, err
	}
	defer response.Body.Close()
	var result struct {
		RunnerID        string `json:"runner_id"`
		NATSURL         string `json:"nats_url"`
		MaxMessageBytes int    `json:"max_message_bytes"`
		Capacity        int    `json:"capacity"`
		Error           string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return RunnerConnection{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if result.Error == "" {
			result.Error = response.Status
		}
		return RunnerConnection{}, errors.New(result.Error)
	}
	if result.RunnerID == "" || result.MaxMessageBytes <= 0 {
		return RunnerConnection{}, errors.New("runner enrollment returned incomplete connection data")
	}
	return RunnerConnection{RunnerID: result.RunnerID, NATSURL: result.NATSURL, MaxMessageBytes: result.MaxMessageBytes, Capacity: result.Capacity, ControlPublicKey: b.ControlPublicKey}, nil
}

func DefaultDataDir() string {
	directory, err := os.UserConfigDir()
	if err != nil || directory == "" {
		directory = os.TempDir()
	}
	return filepath.Join(directory, "glyphflow")
}
