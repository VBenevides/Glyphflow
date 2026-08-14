package api

import (
	"net/http"
	"strings"
)

func (s Server) setSessionCookies(w http.ResponseWriter, tokens AuthTokens) {
	secure := strings.HasPrefix(strings.ToLower(strings.TrimSpace(s.CSRFOrigin)), "https://")
	set := func(name, value, path string) {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	}
	set(accessCookie, tokens.AccessToken, "/")
	set(refreshCookie, tokens.RefreshToken, "/api/v1/auth")
	set(sessionCookie, tokens.SessionID, "/api/v1/auth")
}

func (s Server) setAccessCookie(w http.ResponseWriter, token string) {
	secure := strings.HasPrefix(strings.ToLower(strings.TrimSpace(s.CSRFOrigin)), "https://")
	http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func clearCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (s Server) clearSessionCookies(w http.ResponseWriter) {
	clearCookie(w, accessCookie, "/")
	clearCookie(w, refreshCookie, "/api/v1/auth")
	clearCookie(w, sessionCookie, "/api/v1/auth")
}
