package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnrollmentExpiresAndIsSingleUse(t *testing.T) {
	s := NewInfrastructureService()
	expires := time.Now().Add(15 * time.Minute)
	s.enrollments["token"] = &enrollment{Token: "token", RunnerID: "runner-1", Expires: expires}
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	if _, err := s.ConsumeEnrollment("token", expires.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeEnrollment("token", expires.Add(-time.Second)); !errors.Is(err, errEnrollmentUsed) {
		t.Fatalf("second use returned %v", err)
	}
	s.enrollments["expired"] = &enrollment{Token: "expired", RunnerID: "runner-1", Expires: expires}
	if _, err := s.ConsumeEnrollment("expired", expires); !errors.Is(err, errEnrollmentExpired) {
		t.Fatalf("expired token returned %v", err)
	}
}

func TestInfrastructureMarksStaleRunnersAndFencesLeases(t *testing.T) {
	s := NewInfrastructureService()
	now := time.Now().UTC().Truncate(time.Second)
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", HeartbeatAt: now.Add(-2 * time.Minute).Format(time.RFC3339), ObservedState: "ONLINE"}
	s.resources["resource-1"] = ResourceRecord{ID: "resource-1", Name: "resource-1"}
	s.MarkStale(now, time.Minute)
	if s.runners["runner-1"].ObservedState != "OFFLINE" {
		t.Fatalf("runner state = %q", s.runners["runner-1"].ObservedState)
	}
	lease, err := s.AcquireLease("resource-1", "run-1", time.Minute)
	if err != nil || lease.FencingToken != 1 {
		t.Fatalf("acquire = %#v, %v", lease, err)
	}
	if _, err := s.AcquireLease("resource-1", "run-2", time.Minute); !errors.Is(err, errLeaseConflict) {
		t.Fatalf("active takeover returned %v", err)
	}
	if err := s.ReleaseLease("resource-1", "run-2", lease.FencingToken); !errors.Is(err, errLeaseOwner) {
		t.Fatalf("wrong owner release returned %v", err)
	}
	if err := s.ReleaseLease("resource-1", "run-1", lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	lease, err = s.AcquireLease("resource-1", "run-2", time.Minute)
	if err != nil || lease.FencingToken != 2 {
		t.Fatalf("takeover = %#v, %v", lease, err)
	}
}

func TestResourceDeleteGuardsReferencesAndActiveLease(t *testing.T) {
	s := NewInfrastructureService()
	s.resources["referenced"] = ResourceRecord{ID: "referenced", ActiveReferences: 1}
	s.resources["leased"] = ResourceRecord{ID: "leased", Holder: "run-1", ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339)}
	for _, id := range []string{"referenced", "leased"} {
		response := httptest.NewRecorder()
		s.resourcePath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/resources/"+id, nil))
		if response.Code != http.StatusConflict {
			t.Fatalf("delete %s returned %d", id, response.Code)
		}
	}
	response := httptest.NewRecorder()
	s.resourcePath(response, httptest.NewRequest(http.MethodPost, "/api/v1/resources/free/lease", bytes.NewBufferString(`{"holder":"run-1","ttl_seconds":30}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing resource lease returned %d", response.Code)
	}
}

func TestResourceCreate(t *testing.T) {
	s := NewInfrastructureService()
	response := httptest.NewRecorder()
	s.resourceCollection(response, httptest.NewRequest(http.MethodPost, "/api/v1/resources", bytes.NewBufferString(`{"name":"Build lock"}`)))
	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"Build lock"`)) {
		t.Fatalf("create resource returned %d: %s", response.Code, response.Body.String())
	}
	if len(s.resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(s.resources))
	}
}

func TestRunnerDeleteRemovesRunnerAndEnrollments(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	s.enrollments["token"] = &enrollment{RunnerID: "runner-1"}
	response := httptest.NewRecorder()
	s.runnerPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/runner-1", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete runner returned %d", response.Code)
	}
	if _, ok := s.runners["runner-1"]; ok {
		t.Fatal("runner was not deleted")
	}
	if _, ok := s.enrollments["token"]; ok {
		t.Fatal("runner enrollment was not deleted")
	}
}
