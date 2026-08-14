package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const accessTokenVersion = "gf1"

type accessTokenPayload struct {
	UserID    string `json:"sub"`
	SessionID string `json:"sid"`
	ExpiresAt int64  `json:"exp"`
}

type SessionManager struct {
	mu       sync.RWMutex
	secret   []byte
	sessions map[string]accessTokenPayload
}

func NewSessionManager(secret string) (*SessionManager, error) {
	if len([]byte(secret)) < 32 {
		return nil, errors.New("access token secret must contain at least 32 bytes")
	}
	return &SessionManager{secret: []byte(secret), sessions: make(map[string]accessTokenPayload)}, nil
}

func (m *SessionManager) Issue(userID string, lifetime time.Duration) (string, Claims, error) {
	if strings.TrimSpace(userID) == "" || lifetime <= 0 {
		return "", Claims{}, errors.New("user ID and positive lifetime are required")
	}
	sessionIDBytes := make([]byte, 18)
	if _, err := rand.Read(sessionIDBytes); err != nil {
		return "", Claims{}, err
	}
	payload := accessTokenPayload{
		UserID: userID, SessionID: base64.RawURLEncoding.EncodeToString(sessionIDBytes),
		ExpiresAt: time.Now().Add(lifetime).Unix(),
	}
	token, err := m.sign(payload)
	if err != nil {
		return "", Claims{}, err
	}
	m.mu.Lock()
	m.sessions[payload.SessionID] = payload
	m.mu.Unlock()
	return token, Claims{Subject: userID, UserID: userID, SessionID: payload.SessionID}, nil
}

func (m *SessionManager) IssueForSession(userID, sessionID string, lifetime time.Duration) (string, Claims, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(sessionID) == "" || lifetime <= 0 {
		return "", Claims{}, errors.New("user ID, session ID, and positive lifetime are required")
	}
	payload := accessTokenPayload{UserID: userID, SessionID: sessionID, ExpiresAt: time.Now().Add(lifetime).Unix()}
	token, err := m.sign(payload)
	if err != nil {
		return "", Claims{}, err
	}
	m.mu.Lock()
	m.sessions[sessionID] = payload
	m.mu.Unlock()
	return token, Claims{Subject: userID, UserID: userID, SessionID: sessionID}, nil
}

func (m *SessionManager) RevokeUser(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, id)
		}
	}
}

func (m *SessionManager) Revoke(sessionID string) {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
}

func (m *SessionManager) Authenticator() Authenticator {
	return func(r *http.Request) (Claims, bool) {
		const prefix = "Bearer "
		value := r.Header.Get("Authorization")
		if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
			return Claims{}, false
		}
		payload, ok := m.verify(strings.TrimSpace(value[len(prefix):]))
		if !ok {
			return Claims{}, false
		}
		return Claims{Subject: payload.UserID, UserID: payload.UserID, SessionID: payload.SessionID}, true
	}
}

func (m *SessionManager) sign(payload accessTokenPayload) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	message := accessTokenVersion + "." + base64.RawURLEncoding.EncodeToString(encoded)
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(message))
	return message + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (m *SessionManager) verify(token string) (accessTokenPayload, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != accessTokenVersion {
		return accessTokenPayload{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return accessTokenPayload{}, false
	}
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return accessTokenPayload{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return accessTokenPayload{}, false
	}
	var payload accessTokenPayload
	if json.Unmarshal(raw, &payload) != nil || payload.UserID == "" || payload.SessionID == "" || time.Now().Unix() >= payload.ExpiresAt {
		return accessTokenPayload{}, false
	}
	m.mu.RLock()
	active, ok := m.sessions[payload.SessionID]
	m.mu.RUnlock()
	if !ok || active.UserID != payload.UserID || active.ExpiresAt != payload.ExpiresAt {
		return accessTokenPayload{}, false
	}
	return payload, true
}
