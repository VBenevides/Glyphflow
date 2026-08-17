package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

type runnerRepositoryWithDeleteError struct {
	store.RunnerRepository
	err error
}

func (r runnerRepositoryWithDeleteError) DeletePool(context.Context, string) error { return r.err }

func (r runnerRepositoryWithDeleteError) Delete(context.Context, string) (bool, error) {
	return false, r.err
}

func (r runnerRepositoryWithDeleteError) Archive(context.Context, string) (bool, error) {
	return false, r.err
}

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
		{"capacity", `{"runner_id":"runner-2","platform":"linux","architecture":"amd64","capacity":0}`, "capacity must be at least 1"},
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
	s.SetRunnerControlPlaneURL("http://default.example")
	s.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	if err := os.WriteFile(filepath.Join(directory, "glyphflow-runner-linux-amd64"), []byte("runner-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64","control_plane_url":"http://configured.example/","embedded_nats_endpoint":"nats://embedded:4222"}`))
	request.Host = "control.example:8080"
	response := httptest.NewRecorder()
	s.enroll(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("enrollment returned %d: %s", response.Code, response.Body.String())
	}
	if s.runners["runner-1"].Capacity != 10 {
		t.Fatalf("default runner capacity = %d, want 10", s.runners["runner-1"].Capacity)
	}
	if s.runners["runner-1"].ControlPlaneURL != "http://configured.example" {
		t.Fatalf("runner control plane URL = %q", s.runners["runner-1"].ControlPlaneURL)
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
	if err != nil || bootstrap == nil || bootstrap.RunnerID != "runner-1" || bootstrap.ControlPlaneURL != "http://configured.example" || bootstrap.NATSURL != "nats://embedded:4222" {
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
	secondRequest := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64","capacity":42}`))
	secondRequest.Host = "control.example:8080"
	second := httptest.NewRecorder()
	s.enroll(second, secondRequest)
	if s.runners["runner-1"].Capacity != 42 {
		t.Fatalf("configured runner capacity = %d, want 42", s.runners["runner-1"].Capacity)
	}
	var secondResult struct {
		Artifact string `json:"artifact"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResult); err != nil {
		t.Fatal(err)
	}
	secondRaw, err := base64.StdEncoding.DecodeString(secondResult.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	secondBootstrap, err := worker.UnpackBootstrap(secondRaw)
	if err != nil || secondBootstrap == nil || secondBootstrap.ControlPlaneURL != "http://configured.example" {
		t.Fatalf("second bootstrap = %#v, err=%v", secondBootstrap, err)
	}
	rebind := httptest.NewRecorder()
	s.enrollRunner(rebind, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll", bytes.NewBufferString(`{"runner_id":"runner-1","token":"`+secondBootstrap.Token+`","key_id":"`+keyID+`","public_key":"`+publicKey+`"}`)))
	if rebind.Code != http.StatusOK {
		t.Fatalf("same-key rebind returned %d: %s", rebind.Code, rebind.Body.String())
	}
	thirdRequest := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64"}`))
	thirdRequest.Host = "control.example:8080"
	third := httptest.NewRecorder()
	s.enroll(third, thirdRequest)
	if s.runners["runner-1"].Capacity != 42 {
		t.Fatalf("omitted runner capacity = %d, want 42", s.runners["runner-1"].Capacity)
	}
}

func TestRunnerEnrollmentGeneratesUniqueIDForName(t *testing.T) {
	s := NewInfrastructureService()
	directory := t.TempDir()
	s.SetRunnerBinaryDirectory(directory)
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1<<20)
	s.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	if err := os.WriteFile(filepath.Join(directory, "glyphflow-runner-linux-amd64"), []byte("runner-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.enroll(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_name":"build-agent","platform":"linux","architecture":"amd64"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("enrollment returned %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		RunnerID string `json:"runner_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^build-agent-[0-9a-f]{16}$`).MatchString(result.RunnerID) {
		t.Fatalf("runner id = %q", result.RunnerID)
	}
	if s.runners[result.RunnerID].Name != "build-agent" {
		t.Fatalf("runner name = %q", s.runners[result.RunnerID].Name)
	}
}

func TestArchivedRunnerCanReleasePool(t *testing.T) {
	s := NewInfrastructureService()
	s.pools["pool-1"] = RunnerPoolRecord{ID: "pool-1", Name: "pool-1", Enabled: true}
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1", PoolID: "pool-1"}
	archive := httptest.NewRecorder()
	s.deleteRunner(archive, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/runner-1", nil), "runner-1")
	if archive.Code != http.StatusNoContent {
		t.Fatalf("archive returned %d", archive.Code)
	}
	deletePool := httptest.NewRecorder()
	s.poolPath(deletePool, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/pools/pool-1", nil))
	if deletePool.Code != http.StatusNoContent {
		t.Fatalf("pool delete returned %d: %s", deletePool.Code, deletePool.Body.String())
	}
	if s.runners["runner-1"].PoolID != "" {
		t.Fatalf("archived runner pool = %q", s.runners["runner-1"].PoolID)
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

func TestRunnerArchiveKeepsRunnerAndRemovesEnrollments(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	s.enrollments["token"] = &enrollment{RunnerID: "runner-1"}
	response := httptest.NewRecorder()
	s.runnerPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/runner-1", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete runner returned %d", response.Code)
	}
	if runner := s.runners["runner-1"]; !runner.IsArchived || !runner.IsDeleted {
		t.Fatalf("runner flags = %#v", runner)
	}
	if _, ok := s.enrollments["token"]; ok {
		t.Fatal("runner enrollment was not deleted")
	}
}

func TestRunnerArchiveReportsStorageConflict(t *testing.T) {
	s := NewInfrastructureService()
	s.runnerRepository = runnerRepositoryWithDeleteError{err: store.ErrRunnerHasExecutionHistory}
	response := httptest.NewRecorder()
	s.deleteRunner(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/runner-1", nil), "runner-1")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"error":"runner archival failed"`) {
		t.Fatalf("archive runner response = %d: %s", response.Code, response.Body.String())
	}
}

func TestRunnerPoolDeleteReportsWhenPoolIsInUse(t *testing.T) {
	s := NewInfrastructureService()
	s.runnerRepository = runnerRepositoryWithDeleteError{err: store.ErrRunnerPoolInUse}
	response := httptest.NewRecorder()
	s.poolPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/pools/pool-1", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"error":"runner pool is still in use"`) {
		t.Fatalf("delete pool response = %d: %s", response.Code, response.Body.String())
	}
}

func TestRunnerPoolDeleteReportsTaskVersionReferences(t *testing.T) {
	s := NewInfrastructureService()
	s.runnerRepository = runnerRepositoryWithDeleteError{err: store.ErrRunnerPoolHasTaskVersions}
	response := httptest.NewRecorder()
	s.poolPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/pools/pool-1", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"error":"runner pool is still referenced by task versions"`) {
		t.Fatalf("delete pool response = %d: %s", response.Code, response.Body.String())
	}
}

func TestRunnerCapacityUpdatePublishesControlMessage(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1", Capacity: 1}
	publisher := queue.NewMemory()
	key, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.SetRunnerCapacityPublisher(publisher, key)
	response := httptest.NewRecorder()
	s.runnerPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/runners/runner-1", bytes.NewBufferString(`{"capacity":42}`)))
	if response.Code != http.StatusOK || s.runners["runner-1"].Capacity != 42 {
		t.Fatalf("capacity update = %d %s", response.Code, response.Body.String())
	}
	message, err := publisher.Consume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := protocol.DecodeEnvelope(message.Data)
	if err != nil {
		t.Fatal(err)
	}
	rawPayload, err := envelope.PayloadBytes()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := protocol.DecodeRunnerControlPayload(rawPayload)
	if err != nil || payload.Capacity != 42 || payload.RunnerID != "runner-1" {
		t.Fatalf("control payload = %#v, err=%v", payload, err)
	}
}

func TestRunnerNATSEndpointUpdate(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	response := httptest.NewRecorder()
	s.runnerPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/runners/runner-1", bytes.NewBufferString(`{"nats_endpoint":" nats://vmnet8:4222 "}`)))
	if response.Code != http.StatusOK || s.runners["runner-1"].NATSEndpoint != "nats://vmnet8:4222" {
		t.Fatalf("endpoint update = %d %s", response.Code, response.Body.String())
	}
}

func TestRunnerControlPlaneEndpointUpdate(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	response := httptest.NewRecorder()
	s.runnerPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/runners/runner-1", bytes.NewBufferString(`{"control_plane_url":" http://control.example/ "}`)))
	if response.Code != http.StatusOK || s.runners["runner-1"].ControlPlaneURL != "http://control.example" {
		t.Fatalf("control plane URL update = %d %s", response.Code, response.Body.String())
	}
}
