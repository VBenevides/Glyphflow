package api

import "net/http"

func (s Server) runtimeConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	config := s.Config
	if config.Brand == "" {
		config.Brand = "Glyphflow"
	}
	if config.CSRFCookie == "" {
		config.CSRFCookie = "glyphflow_csrf"
	}
	if s.AuthService != nil {
		config.PasswordLogin = s.AuthService.PasswordLoginEnabled()
		config.Registration = s.AuthService.RegistrationEnabled()
	}
	if s.PasswordAuth != nil {
		config.PasswordLogin = s.PasswordAuth.Enabled()
		config.Registration = s.PasswordAuth.RegistrationEnabled()
	}
	config.OIDC = s.OIDC != nil && s.OIDC.EnabledCount() > 0
	token, err := NewCSRFToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "configuration unavailable"})
		return
	}
	setCSRFCookie(w, s.CSRFOrigin, token)
	writeJSON(w, http.StatusOK, config)
}

func setCSRFCookie(w http.ResponseWriter, origin, token string) {
	secure := len(origin) >= len("https://") && origin[:len("https://")] == "https://"
	http.SetCookie(w, &http.Cookie{Name: "glyphflow_csrf", Value: token, Path: "/", Secure: secure, SameSite: http.SameSiteLaxMode})
}
