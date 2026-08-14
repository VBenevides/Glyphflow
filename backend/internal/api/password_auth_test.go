package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPasswordEndpointsRegisterAndLogin(t *testing.T) {
	sessions, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	server := Server{PasswordAuth: NewPasswordAuthService(true, true, nil), Sessions: sessions}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"user","password":"password"}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"user","password":"password"}`))
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
}
