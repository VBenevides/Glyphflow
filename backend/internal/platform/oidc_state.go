package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type AuthorizationStateStore struct {
	mu     sync.Mutex
	states map[string]authorizationState
	key    []byte
}

type authorizationState struct {
	Provider string
	Purpose  string
	Nonce    string
	Callback string
	Verifier []byte
	Expires  time.Time
	Used     bool
}

type AuthorizationStateSnapshot struct {
	Provider string    `json:"provider"`
	Purpose  string    `json:"purpose"`
	Nonce    string    `json:"nonce"`
	Callback string    `json:"callback"`
	Verifier []byte    `json:"verifier"`
	Expires  time.Time `json:"expires"`
	Used     bool      `json:"used"`
}

type AuthorizationStoreSnapshot struct {
	Key    []byte                                `json:"key"`
	States map[string]AuthorizationStateSnapshot `json:"states"`
}

func (s *AuthorizationStateStore) Snapshot() AuthorizationStoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	states := make(map[string]AuthorizationStateSnapshot, len(s.states))
	for key, state := range s.states {
		states[key] = AuthorizationStateSnapshot{Provider: state.Provider, Purpose: state.Purpose, Nonce: state.Nonce, Callback: state.Callback, Verifier: append([]byte(nil), state.Verifier...), Expires: state.Expires, Used: state.Used}
	}
	return AuthorizationStoreSnapshot{Key: append([]byte(nil), s.key...), States: states}
}

func (s *AuthorizationStateStore) Restore(snapshot AuthorizationStoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = append([]byte(nil), snapshot.Key...)
	s.states = make(map[string]authorizationState, len(snapshot.States))
	for key, state := range snapshot.States {
		s.states[key] = authorizationState{Provider: state.Provider, Purpose: state.Purpose, Nonce: state.Nonce, Callback: state.Callback, Verifier: append([]byte(nil), state.Verifier...), Expires: state.Expires, Used: state.Used}
	}
}

func NewAuthorizationStateStore() *AuthorizationStateStore {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return &AuthorizationStateStore{states: make(map[string]authorizationState), key: key}
}

func (s *AuthorizationStateStore) Create(provider, purpose string, now time.Time, lifetime time.Duration) (string, string, error) {
	if provider == "" || purpose == "" || lifetime <= 0 {
		return "", "", errors.New("authorization state inputs are required")
	}
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	state := hex.EncodeToString(value)
	nonce := hex.EncodeToString(value[:16])
	s.mu.Lock()
	s.states[hashString(state)] = authorizationState{Provider: provider, Purpose: purpose, Nonce: hashString(nonce), Expires: now.Add(lifetime)}
	s.mu.Unlock()
	return state, nonce, nil
}

func (s *AuthorizationStateStore) CreateChallenge(provider, purpose, callback, verifier string, now time.Time, lifetime time.Duration) (string, string, error) {
	if callback == "" || verifier == "" {
		return "", "", errors.New("callback and PKCE verifier are required")
	}
	state, nonce, err := s.Create(provider, purpose, now, lifetime)
	if err != nil {
		return "", "", err
	}
	ciphertext, err := s.encrypt([]byte(verifier))
	if err != nil {
		return "", "", err
	}
	s.mu.Lock()
	entry := s.states[hashString(state)]
	entry.Callback, entry.Verifier = callback, ciphertext
	s.states[hashString(state)] = entry
	s.mu.Unlock()
	return state, nonce, nil
}

func (s *AuthorizationStateStore) Consume(state, nonce, provider, purpose string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := hashString(state)
	entry, ok := s.states[key]
	if !ok || entry.Used || entry.Provider != provider || entry.Purpose != purpose || !now.Before(entry.Expires) || entry.Nonce != hashString(nonce) {
		return errors.New("authorization state is invalid")
	}
	entry.Used = true
	s.states[key] = entry
	return nil
}

func (s *AuthorizationStateStore) ConsumeChallenge(state, nonce, provider, purpose, callback, verifier string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := hashString(state)
	entry, ok := s.states[key]
	if !ok || entry.Used || entry.Provider != provider || entry.Purpose != purpose || entry.Callback != callback || !now.Before(entry.Expires) || entry.Nonce != hashString(nonce) {
		return errors.New("authorization state is invalid")
	}
	plain, err := s.decrypt(entry.Verifier)
	if err != nil || string(plain) != verifier {
		return errors.New("PKCE verifier is invalid")
	}
	entry.Used = true
	s.states[key] = entry
	return nil
}

func (s *AuthorizationStateStore) ReadChallenge(state, nonce, purpose string, now time.Time) (provider, callback, verifier string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := hashString(state)
	entry, ok := s.states[key]
	if !ok || entry.Used || entry.Purpose != purpose || !now.Before(entry.Expires) || entry.Nonce != hashString(nonce) {
		return "", "", "", errors.New("authorization state is invalid")
	}
	plain, err := s.decrypt(entry.Verifier)
	if err != nil {
		return "", "", "", errors.New("PKCE verifier is invalid")
	}
	return entry.Provider, entry.Callback, string(plain), nil
}

func (s *AuthorizationStateStore) encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}
func (s *AuthorizationStateStore) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("invalid verifier ciphertext")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func hashString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
