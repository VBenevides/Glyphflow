package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type PasswordAuthService struct {
	mu                    sync.Mutex
	hasher                platform.PasswordHasher
	users                 map[string]string
	enabled, registration bool
}

func NewPasswordAuthService(enabled, registration bool, pepper []byte) *PasswordAuthService {
	return &PasswordAuthService{hasher: platform.DefaultPasswordHasher(pepper), users: map[string]string{}, enabled: enabled, registration: registration}
}
func (s *PasswordAuthService) Register(username, password string) error {
	if !s.enabled || !s.registration {
		return errors.New("password registration is disabled")
	}
	if username == "" || password == "" {
		return errors.New("username and password are required")
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := platform.NormalizeIdentityKey(username)
	if _, ok := s.users[key]; ok {
		return errors.New("user already exists")
	}
	s.users[key] = hash
	return nil
}
func (s *PasswordAuthService) Verify(username, password string) bool {
	if !s.enabled {
		return false
	}
	s.mu.Lock()
	hash := s.users[platform.NormalizeIdentityKey(username)]
	s.mu.Unlock()
	if hash == "" {
		return false
	}
	ok, err := s.hasher.Verify(hash, password)
	return err == nil && ok
}

type passwordRequest struct{ Username, Password string }

func (s Server) passwordRoutes(mux *http.ServeMux) {
	if s.AuthService != nil {
		mux.HandleFunc("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in passwordRequest
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				writeJSON(w, 400, map[string]string{"error": "registration failed"})
				return
			}
			user, err := s.AuthService.Register(in.Username, in.Password)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "registration failed"})
				return
			}
			writeJSON(w, 201, map[string]string{"id": user.ID, "username": user.Username})
		})
		mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in passwordRequest
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
				return
			}
			tokens, err := s.AuthService.Login(in.Username, in.Password)
			if err != nil {
				writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
				return
			}
			writeJSON(w, 200, tokens)
		})
		mux.HandleFunc("/api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in struct{ SessionID, RefreshToken string }
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				writeJSON(w, 401, map[string]string{"error": "invalid refresh"})
				return
			}
			tokens, err := s.AuthService.Refresh(in.SessionID, in.RefreshToken)
			if err != nil {
				writeJSON(w, 401, map[string]string{"error": "invalid refresh"})
				return
			}
			writeJSON(w, 200, tokens)
		})
		mux.HandleFunc("/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in struct{ SessionID string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			s.AuthService.Logout(in.SessionID)
			writeJSON(w, 204, nil)
		})
		mux.HandleFunc("/api/v1/auth/logout-all", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			claims, ok := s.AuthService.Authenticator()(r)
			if !ok {
				writeJSON(w, 401, map[string]string{"error": "authentication required"})
				return
			}
			s.AuthService.LogoutAll(claims.UserID)
			writeJSON(w, 204, nil)
		})
		return
	}
	if s.PasswordAuth == nil {
		return
	}
	mux.HandleFunc("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var in passwordRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil || s.PasswordAuth.Register(in.Username, in.Password) != nil {
			writeJSON(w, 400, map[string]string{"error": "registration failed"})
			return
		}
		writeJSON(w, 201, map[string]string{"status": "created"})
	})
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var in passwordRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil || !s.PasswordAuth.Verify(in.Username, in.Password) || s.Sessions == nil {
			writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
			return
		}
		token, _, err := s.Sessions.Issue(platform.NormalizeIdentityKey(in.Username), time.Hour)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "login failed"})
			return
		}
		writeJSON(w, 200, map[string]string{"access_token": token})
	})
}
