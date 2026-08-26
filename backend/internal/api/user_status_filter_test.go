package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserStatusFilteringAndPagination(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	_, err = auth.Register("active@example.com", "correct horse active")
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := auth.Register("disabled@example.com", "correct horse disabled")
	if err != nil {
		t.Fatal(err)
	}
	auth.SetUserApprovalRequired(true)
	if _, err := auth.Register("pending-one@example.com", "correct horse pending 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Register("pending-two@example.com", "correct horse pending 2"); err != nil {
		t.Fatal(err)
	}
	if err := auth.DisableUser(disabled.ID); err != nil {
		t.Fatal(err)
	}

	handler := (Server{
		AuthService: auth,
		AuthAdmin:   &AuthAdminService{Auth: auth},
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{Roles: map[string]bool{"users.read": true}}, true
		},
	}).Handler()
	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, bytes.NewBuffer(nil)))
		return response
	}
	decode := func(response *httptest.ResponseRecorder) (items []map[string]any, total, pages int) {
		var page struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
			Pages int              `json:"pages"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page.Items, page.Total, page.Pages
	}

	for _, test := range []struct {
		name, query, status string
		total, pages        int
	}{
		{name: "active", query: "status=active", status: "active", total: 1, pages: 1},
		{name: "pending", query: "status=pending&limit=1", status: "pending", total: 2, pages: 2},
		{name: "disabled", query: "status=disabled", status: "disabled", total: 1, pages: 1},
		{name: "combined", query: "status=pending&email=pending-two", status: "pending", total: 1, pages: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := request("/api/v1/users?" + test.query)
			if response.Code != http.StatusOK {
				t.Fatalf("status: %d %s", response.Code, response.Body.String())
			}
			items, total, pages := decode(response)
			if total != test.total || pages != test.pages || len(items) == 0 {
				t.Fatalf("page: items=%d total=%d pages=%d", len(items), total, pages)
			}
			for _, item := range items {
				if item["status"] != test.status {
					t.Fatalf("status filter returned %#v", item)
				}
			}
		})
	}

	if response := request("/api/v1/users?status=unknown"); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid status: %d %s", response.Code, response.Body.String())
	}
}
