package api

import (
	"net/http"
	"strings"
)

func (s Server) setSessionCookies(w http.ResponseWriter, tokens AuthTokens) {
	secure := secureCookies(s.CSRFOrigin)
	set := func(name, value, path string) {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	}
	set(accessCookie, tokens.AccessToken, "/")
	set(refreshCookie, tokens.RefreshToken, "/api/v1/auth")
	set(sessionCookie, tokens.SessionID, "/api/v1/auth")
}

func (s Server) setAccessCookie(w http.ResponseWriter, token string) {
	secure := secureCookies(s.CSRFOrigin)
	http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func clearCookie(w http.ResponseWriter, name, path string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func (s Server) clearSessionCookies(w http.ResponseWriter) {
	secure := secureCookies(s.CSRFOrigin)
	clearCookie(w, accessCookie, "/", secure)
	clearCookie(w, refreshCookie, "/api/v1/auth", secure)
	clearCookie(w, sessionCookie, "/api/v1/auth", secure)
	// The CSRF cookie must remain readable by the frontend for double-submit validation.
	http.SetCookie(w, &http.Cookie{Name: "glyphflow_csrf", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func secureCookies(origin string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(origin)), "https://")
}
