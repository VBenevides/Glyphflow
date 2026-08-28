package api

import (
	"context"
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

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const accessTokenVersion = "gf1"

const (
	accessCookie  = "glyphflow_access"
	refreshCookie = "glyphflow_refresh"
	sessionCookie = "glyphflow_session"
)

type accessTokenPayload struct {
	UserID    string `json:"sub"`
	SessionID string `json:"sid"`
	ExpiresAt int64  `json:"exp"`
}

type SessionManager struct {
	mu         sync.RWMutex
	secret     []byte
	sessions   map[string]accessTokenPayload
	repository store.SessionRepository
}
type SessionInfo struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	UserAgent  string     `json:"userAgent,omitempty"`
	IPAddress  string     `json:"ipAddress,omitempty"`
	Current    bool       `json:"current,omitempty"`
}

type AdminSession struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	UserEmail  string     `json:"userEmail"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	UserAgent  string     `json:"userAgent,omitempty"`
	IPAddress  string     `json:"ipAddress,omitempty"`
}

func (m *SessionManager) Owns(userID, sessionID string) bool {
	if m.repository != nil {
		session, ok, err := m.repository.Get(context.Background(), sessionID)
		return err == nil && ok && session.UserID == userID && session.RevokedAt == nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	return ok && session.UserID == userID
}

func (m *SessionManager) SetRepository(repository store.SessionRepository) {
	m.mu.Lock()
	m.repository = repository
	m.mu.Unlock()
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
	if m.repository == nil {
		m.sessions[payload.SessionID] = payload
	}
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
	if m.repository == nil {
		m.sessions[sessionID] = payload
	}
	m.mu.Unlock()
	return token, Claims{Subject: userID, UserID: userID, SessionID: sessionID}, nil
}

func (m *SessionManager) RevokeUser(userID string) error {
	if m.repository != nil {
		return m.repository.RevokeUser(context.Background(), userID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, id)
		}
	}
	return nil
}

func (m *SessionManager) List(userID string) ([]SessionInfo, error) {
	if m.repository != nil {
		sessions, err := m.repository.List(context.Background(), userID)
		if err != nil {
			return nil, err
		}
		out := make([]SessionInfo, 0, len(sessions))
		for _, session := range sessions {
			lastSeenAt := session.LastSeenAt
			out = append(out, SessionInfo{ID: session.ID, UserID: userID, ExpiresAt: session.AccessExpiresAt, LastSeenAt: &lastSeenAt, UserAgent: session.UserAgent, IPAddress: session.IPAddress})
		}
		return out, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []SessionInfo{}
	for id, session := range m.sessions {
		if session.UserID == userID {
			out = append(out, SessionInfo{ID: id, UserID: userID, ExpiresAt: time.Unix(session.ExpiresAt, 0)})
		}
	}
	return out, nil
}

func (m *SessionManager) AdminPage(email string, limit, offset int) ([]AdminSession, int, bool, error) {
	m.mu.RLock()
	repository := m.repository
	m.mu.RUnlock()
	paged, ok := repository.(interface {
		ListAdminPage(context.Context, string, int, int) ([]store.AdminSessionRecord, int, error)
	})
	if !ok {
		return nil, 0, false, nil
	}
	records, total, err := paged.ListAdminPage(context.Background(), email, limit, offset)
	if err != nil {
		return nil, 0, true, err
	}
	sessions := make([]AdminSession, 0, len(records))
	for _, record := range records {
		lastSeenAt := record.LastSeenAt
		sessions = append(sessions, AdminSession{ID: record.ID, UserID: record.UserID, UserEmail: record.UserEmail, ExpiresAt: record.ExpiresAt, LastSeenAt: &lastSeenAt, UserAgent: record.UserAgent, IPAddress: record.IPAddress})
	}
	return sessions, total, true, nil
}

func (m *SessionManager) Revoke(sessionID string) error {
	if m.repository != nil {
		return m.repository.Revoke(context.Background(), sessionID)
	}
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	return nil
}

func (m *SessionManager) Authenticator() Authenticator {
	return func(r *http.Request) (Claims, bool) {
		value := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(value) > len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) {
			value = strings.TrimSpace(value[len(prefix):])
		} else if cookie, err := r.Cookie(accessCookie); err == nil {
			value = cookie.Value
		} else {
			return Claims{}, false
		}
		payload, ok := m.verify(value)
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
	repository := m.repository
	active, ok := m.sessions[payload.SessionID]
	m.mu.RUnlock()
	if repository != nil {
		activeInDatabase, err := repository.Active(context.Background(), payload.SessionID, payload.UserID)
		if err != nil || !activeInDatabase {
			return accessTokenPayload{}, false
		}
	} else if !ok || active.UserID != payload.UserID || active.ExpiresAt != payload.ExpiresAt {
		return accessTokenPayload{}, false
	}
	return payload, true
}
