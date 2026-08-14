package api

import (
	"net"
	"net/http"
	"strings"
	"time"
)

func (s Server) allowAuth(w http.ResponseWriter, r *http.Request, key string) bool {
	if s.AuthRateLimiter == nil || s.AuthRateLimiter.Allow(key+"|"+requestSource(r), time.Now()) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many authentication attempts"})
	return false
}

func requestSource(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
