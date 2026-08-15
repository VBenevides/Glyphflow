package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/worker"
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

func TestRunnerEnrollmentExplainsInvalidTarget(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"runner ID", `{"runner_id":"Runner 2","platform":"linux","architecture":"amd64"}`, "runner_id must contain only letters, digits, dot, underscore, or hyphen"},
		{"platform", `{"runner_id":"runner-2","platform":"darwin","architecture":"amd64"}`, "platform must be linux or windows"},
		{"architecture", `{"runner_id":"runner-2","platform":"linux","architecture":"arm64"}`, "architecture must be amd64"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewInfrastructureService().enroll(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(test.body)))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRunnerEnrollmentExplainsArtifactFailure(t *testing.T) {
	s := NewInfrastructureService()
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1<<20)
	response := httptest.NewRecorder()
	s.enroll(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64"}`)))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"error":"runner binary is unavailable"`) || strings.Contains(response.Body.String(), "runner binary is unavailable:") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRunnerEnrollmentExplainsTokenFailure(t *testing.T) {
	s := NewInfrastructureService()
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1<<20)
	s.enrollments["token"] = &enrollment{RunnerID: "runner-1", Expires: time.Now().Add(-time.Minute)}
	response := httptest.NewRecorder()
	s.enrollRunner(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll", bytes.NewBufferString(`{"runner_id":"runner-1","token":"token","key_id":"runner:runner-1","public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"error":"runner enrollment rejected"`) || strings.Contains(response.Body.String(), "enrollment expired") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRunnerEnrollmentBuildsBootstrapBinaryAndConsumesToken(t *testing.T) {
	s := NewInfrastructureService()
	directory := t.TempDir()
	s.SetRunnerBinaryDirectory(directory)
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1<<20)
	s.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	if err := os.WriteFile(filepath.Join(directory, "glyphflow-runner-linux-amd64"), []byte("runner-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64"}`))
	request.Host = "control.example:8080"
	response := httptest.NewRecorder()
	s.enroll(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("enrollment returned %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Artifact string `json:"artifact"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Filename != "runner-1-glyphflow-runner-linux-amd64" {
		t.Fatalf("filename = %q", result.Filename)
	}
	raw, err := base64.StdEncoding.DecodeString(result.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := worker.UnpackBootstrap(raw)
	if err != nil || bootstrap == nil || bootstrap.RunnerID != "runner-1" || bootstrap.ControlPlaneURL != "http://control.example:8080" || bootstrap.NATSURL != "nats://localhost:4222" {
		t.Fatalf("bootstrap = %#v, err=%v", bootstrap, err)
	}
	consume := httptest.NewRecorder()
	keyID := "runner:runner-1"
	publicKey := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	s.enrollRunner(consume, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll", bytes.NewBufferString(`{"runner_id":"runner-1","token":"`+bootstrap.Token+`","key_id":"`+keyID+`","public_key":"`+publicKey+`"}`)))
	if consume.Code != http.StatusOK {
		t.Fatalf("consumption returned %d: %s", consume.Code, consume.Body.String())
	}
	replay := httptest.NewRecorder()
	s.enrollRunner(replay, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll", bytes.NewBufferString(`{"runner_id":"runner-1","token":"`+bootstrap.Token+`","key_id":"`+keyID+`","public_key":"`+publicKey+`"}`)))
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay returned %d", replay.Code)
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

func TestFilterRunnersByObservedState(t *testing.T) {
	items := filterRunners([]RunnerRecord{
		{ID: "runner-1", ObservedState: "ONLINE"},
		{ID: "runner-2", ObservedState: "OFFLINE"},
	}, "offline")
	if len(items) != 1 || items[0].ID != "runner-2" {
		t.Fatalf("offline runners = %#v", items)
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
