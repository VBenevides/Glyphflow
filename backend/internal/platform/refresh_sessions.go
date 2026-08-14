package platform

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type RefreshSessionManager struct {
	mu       sync.Mutex
	sessions map[string]refreshSession
	disabled map[string]bool
}

type refreshSession struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time
}

type RefreshSessionSnapshot struct {
	UserID    string    `json:"userId"`
	TokenHash string    `json:"tokenHash"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (m *RefreshSessionManager) Snapshot() (map[string]RefreshSessionSnapshot, map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessions := make(map[string]RefreshSessionSnapshot, len(m.sessions))
	for id, session := range m.sessions {
		sessions[id] = RefreshSessionSnapshot{UserID: session.UserID, TokenHash: session.TokenHash, ExpiresAt: session.ExpiresAt}
	}
	disabled := make(map[string]bool, len(m.disabled))
	for userID, value := range m.disabled {
		disabled[userID] = value
	}
	return sessions, disabled
}

func (m *RefreshSessionManager) Restore(sessions map[string]RefreshSessionSnapshot, disabled map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]refreshSession, len(sessions))
	for id, session := range sessions {
		m.sessions[id] = refreshSession{UserID: session.UserID, TokenHash: session.TokenHash, ExpiresAt: session.ExpiresAt}
	}
	m.disabled = make(map[string]bool, len(disabled))
	for userID, value := range disabled {
		m.disabled[userID] = value
	}
}

func NewRefreshSessionManager() *RefreshSessionManager {
	return &RefreshSessionManager{sessions: make(map[string]refreshSession), disabled: make(map[string]bool)}
}

func (m *RefreshSessionManager) Issue(userID string, lifetime time.Duration) (sessionID, token string, err error) {
	if userID == "" || lifetime <= 0 {
		return "", "", errors.New("user ID and positive lifetime are required")
	}
	sessionID, token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	m.mu.Lock()
	m.sessions[sessionID] = refreshSession{UserID: userID, TokenHash: HashToken(token), ExpiresAt: time.Now().Add(lifetime)}
	m.mu.Unlock()
	return sessionID, token, nil
}

func (m *RefreshSessionManager) Rotate(sessionID, token string, lifetime time.Duration) (newID, newToken string, err error) {
	if sessionID == "" || token == "" || lifetime <= 0 {
		return "", "", errors.New("refresh session inputs are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok || m.disabled[session.UserID] || !time.Now().Before(session.ExpiresAt) || session.TokenHash != HashToken(token) {
		return "", "", errors.New("refresh token is invalid")
	}
	newID, newToken, err = randomToken()
	if err != nil {
		return "", "", err
	}
	// Delete before issuing the replacement. A replay of the old token cannot win a race.
	delete(m.sessions, sessionID)
	m.sessions[newID] = refreshSession{UserID: session.UserID, TokenHash: HashToken(newToken), ExpiresAt: time.Now().Add(lifetime)}
	return newID, newToken, nil
}

func (m *RefreshSessionManager) Revoke(sessionID string) {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
}

func (m *RefreshSessionManager) UserID(sessionID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	return session.UserID, ok
}

func (m *RefreshSessionManager) RevokeUser(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, id)
		}
	}
}

func (m *RefreshSessionManager) DisableUser(userID string) {
	m.mu.Lock()
	m.disabled[userID] = true
	for id, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()
}

func randomToken() (string, string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	first := hex.EncodeToString(value[:16])
	second := hex.EncodeToString(value[16:])
	return first, second, nil
}
