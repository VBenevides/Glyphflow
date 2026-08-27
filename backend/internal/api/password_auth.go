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

func (s *PasswordAuthService) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *PasswordAuthService) RegistrationEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registration && s.enabled
}
func (s *PasswordAuthService) Register(email, password string) error {
	if !s.enabled || !s.registration {
		return errors.New("password registration is disabled")
	}
	email, err := platform.NormalizeEmail(email)
	if err != nil || password == "" {
		return errors.New("email and password are required")
	}
	if err := platform.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := email
	if _, ok := s.users[key]; ok {
		return errors.New("user already exists")
	}
	s.users[key] = hash
	return nil
}
func (s *PasswordAuthService) Verify(email, password string) bool {
	if !s.enabled {
		return false
	}
	email, err := platform.NormalizeEmail(email)
	if err != nil {
		return false
	}
	s.mu.Lock()
	hash := s.users[email]
	s.mu.Unlock()
	if hash == "" {
		return false
	}
	ok, err := s.hasher.Verify(hash, password)
	return err == nil && ok
}

type passwordRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s Server) passwordRoutes(mux routeRegistrar) {
	if s.AuthService != nil {
		mux.HandleFunc("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in passwordRequest
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "invalid registration request", err)
				return
			}
			if !s.allowAuth(w, r, "password-register|"+platform.NormalizeIdentityKey(in.Email)) {
				return
			}
			user, err := s.AuthService.Register(in.Email, in.Password)
			if err != nil {
				writeError(w, http.StatusBadRequest, "registration failed", err)
				return
			}
			writeJSON(w, 201, map[string]string{"id": user.ID, "email": user.Email, "status": user.Status})
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
			if !s.allowAuth(w, r, "password-login|"+platform.NormalizeIdentityKey(in.Email)) {
				return
			}
			tokens, err := s.AuthService.Login(in.Email, in.Password)
			if err != nil {
				if errors.Is(err, ErrPendingUser) {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
				return
			}
			s.setSessionCookies(w, tokens)
			writeJSON(w, 200, map[string]string{"status": "authenticated"})
		})
		mux.HandleFunc("/api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in struct{ SessionID, RefreshToken string }
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				in = struct{ SessionID, RefreshToken string }{}
			}
			if in.SessionID == "" {
				if cookie, err := r.Cookie(sessionCookie); err == nil {
					in.SessionID = cookie.Value
				}
			}
			if in.RefreshToken == "" {
				if cookie, err := r.Cookie(refreshCookie); err == nil {
					in.RefreshToken = cookie.Value
				}
			}
			tokens, err := s.AuthService.Refresh(in.SessionID, in.RefreshToken)
			if err != nil {
				writeJSON(w, 401, map[string]string{"error": "invalid refresh"})
				return
			}
			s.setSessionCookies(w, tokens)
			writeJSON(w, 200, map[string]string{"status": "refreshed"})
		})
		mux.HandleFunc("/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			var in struct{ SessionID string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			if claims, ok := s.AuthService.Authenticator()(r); ok {
				in.SessionID = claims.SessionID
			}
			if err := s.AuthService.Logout(in.SessionID); err != nil {
				writeError(w, http.StatusInternalServerError, "logout failed", err)
				return
			}
			s.clearSessionCookies(w)
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
			if err := s.AuthService.LogoutAll(claims.UserID); err != nil {
				writeError(w, http.StatusInternalServerError, "logout-all failed", err)
				return
			}
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
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid registration request", err)
			return
		}
		if !s.allowAuth(w, r, "password-register|"+platform.NormalizeIdentityKey(in.Email)) {
			return
		}
		if err := s.PasswordAuth.Register(in.Email, in.Password); err != nil {
			writeError(w, http.StatusBadRequest, "registration failed", err)
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
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
			return
		}
		if !s.allowAuth(w, r, "password-login|"+platform.NormalizeIdentityKey(in.Email)) {
			return
		}
		if !s.PasswordAuth.Verify(in.Email, in.Password) || s.Sessions == nil {
			writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
			return
		}
		email, err := platform.NormalizeEmail(in.Email)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
			return
		}
		token, _, err := s.Sessions.Issue(email, time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "login failed", err)
			return
		}
		s.setAccessCookie(w, token)
		writeJSON(w, 200, map[string]string{"status": "authenticated"})
	})
}
