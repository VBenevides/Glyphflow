package api

import "net/http"

func (s Server) secretAttention(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.OIDC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "secret status unavailable"})
		return
	}
	secrets, err := s.OIDC.SecretAttention()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "secret status unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, secrets)
}
