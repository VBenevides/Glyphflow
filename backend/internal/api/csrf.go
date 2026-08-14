package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

func NewCSRFToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func ValidateCSRFRequest(r *http.Request, expectedOrigin string) error {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return nil
	}
	if expectedOrigin == "" {
		return errors.New("CSRF origin is not configured")
	}
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	if origin != expectedOrigin {
		return errors.New("CSRF origin is not allowed")
	}
	cookie, err := r.Cookie("glyphflow_csrf")
	if err != nil || cookie.Value == "" {
		return errors.New("CSRF cookie is missing")
	}
	header := r.Header.Get("X-CSRF-Token")
	if len(header) != len(cookie.Value) || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
		return errors.New("CSRF token is invalid")
	}
	return nil
}

func (s Server) withCSRF(next http.Handler, expectedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs/login" || r.URL.Path == "/api/v1/runners/enroll" {
			// This helper returns a bearer token in the response body and creates no cookie session.
			next.ServeHTTP(w, r)
			return
		}
		if err := ValidateCSRFRequest(r, expectedOrigin); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request rejected"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
