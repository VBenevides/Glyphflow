package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPermissionsResolvePerRequest(t *testing.T) {
	granted := true
	h := (Server{
		Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "u"}, true },
		Permissions: func(Claims) map[string]bool {
			if granted {
				return map[string]bool{"task.read": true}
			}
			return nil
		},
	}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == http.StatusForbidden {
		t.Fatal("permission unexpectedly denied")
	}
	granted = false
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("revoked permission returned %d", w.Code)
	}
}
