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
		{"ui", `{"runner_id":"runner-2","platform":"linux","architecture":"amd64","ui":"unknown"}`, "ui must be gui, tui, or headless"},
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

func TestRunnerArtifactRejectsUnsafeTargetComponents(t *testing.T) {
	s := NewInfrastructureService()
	s.SetRunnerBinaryDirectory(t.TempDir())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", nil)
	cases := []struct {
		name, platform, architecture string
	}{
		{"platform traversal", "../linux", "amd64"},
		{"platform slash", "linux/other", "amd64"},
		{"platform backslash", `linux\other`, "amd64"},
		{"platform absolute", "/tmp", "amd64"},
		{"platform unknown", "darwin", "amd64"},
		{"architecture traversal", "linux", "../amd64"},
		{"architecture slash", "linux", "amd64/other"},
		{"architecture backslash", "linux", `amd64\other`},
		{"architecture absolute", "linux", "/tmp"},
		{"architecture unknown", "linux", "arm64"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := s.buildRunnerArtifact(request, test.platform, test.architecture, "runner-1", "token", "", "", "gui")
			if err == nil || err.Error() != "runner artifact target is invalid" {
				t.Fatalf("buildRunnerArtifact() error = %v", err)
			}
		})
	}
}

func TestRunnerArtifactAcceptsSupportedTargets(t *testing.T) {
	s := NewInfrastructureService()
	directory := t.TempDir()
	s.SetRunnerBinaryDirectory(directory)
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1<<20)
	s.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", nil)
	for _, test := range []struct {
		platform, filename string
	}{
		{"linux", "glyphflow-runner-linux-amd64"},
		{"windows", "glyphflow-runner-windows-amd64.exe"},
	} {
		t.Run(test.platform, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(directory, test.filename), []byte("runner-binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			_, filename, err := s.buildRunnerArtifact(request, test.platform, "amd64", "runner-1", "token", "http://control.example", "nats://localhost:4222", "gui")
			if err != nil || filename != "runner-1-"+test.filename {
				t.Fatalf("buildRunnerArtifact() = %q, %v", filename, err)
			}
		})
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

func TestRunnerEndpointPolicyRequiresApprovedTLSInProduction(t *testing.T) {
	if err := validateRunnerEndpoints("http://control.example", "nats://broker:4222", "https://control.example", "tls://broker:4222", false); err == nil {
		t.Fatal("insecure runner endpoints were accepted")
	}
	if err := validateRunnerEndpoints("https://other.example", "tls://broker:4222", "https://control.example", "tls://broker:4222", false); err == nil {
		t.Fatal("unapproved control-plane endpoint was accepted")
	}
	if err := validateRunnerEndpoints("https://control.example", "tls://broker:4222", "https://control.example", "tls://broker:4222", false); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerBootstrapEnrollmentIsRateLimited(t *testing.T) {
	s := NewInfrastructureService()
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1<<20)
	for attempt := 0; attempt < 10; attempt++ {
		response := httptest.NewRecorder()
		s.enrollRunner(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll", bytes.NewBufferString(`{"runner_id":"runner-1","token":"bad","key_id":"runner:runner-1","public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)))
		if response.Code == http.StatusTooManyRequests {
			t.Fatalf("rate limit triggered too early on attempt %d", attempt+1)
		}
	}
	response := httptest.NewRecorder()
	s.enrollRunner(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll", bytes.NewBufferString(`{"runner_id":"runner-1","token":"bad","key_id":"runner:runner-1","public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit status = %d", response.Code)
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
	if err := os.WriteFile(filepath.Join(directory, "glyphflow-runner-linux-amd64-tui"), []byte("tui-runner-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "glyphflow-runner-linux-amd64-headless"), []byte("headless-runner-binary"), 0o700); err != nil {
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
	if err != nil || bootstrap == nil || bootstrap.RunnerID != "runner-1" || bootstrap.ControlPlaneURL != "http://configured.example" || bootstrap.NATSURL != "nats://embedded:4222" || !bootstrap.AllowInsecureTransport {
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
	secondRequest := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64","capacity":42,"ui":"tui"}`))
	secondRequest.Host = "control.example:8080"
	second := httptest.NewRecorder()
	s.enroll(second, secondRequest)
	if s.runners["runner-1"].Capacity != 42 {
		t.Fatalf("configured runner capacity = %d, want 42", s.runners["runner-1"].Capacity)
	}
	var secondResult struct {
		Artifact string `json:"artifact"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResult); err != nil {
		t.Fatal(err)
	}
	if secondResult.Filename != "runner-1-glyphflow-runner-linux-amd64-tui" {
		t.Fatalf("TUI filename = %q", secondResult.Filename)
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
	thirdRequest := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", bytes.NewBufferString(`{"runner_id":"runner-1","platform":"linux","architecture":"amd64","ui":"headless"}`))
	thirdRequest.Host = "control.example:8080"
	third := httptest.NewRecorder()
	s.enroll(third, thirdRequest)
	if s.runners["runner-1"].Capacity != 42 {
		t.Fatalf("omitted runner capacity = %d, want 42", s.runners["runner-1"].Capacity)
	}
	var thirdResult struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(third.Body.Bytes(), &thirdResult); err != nil {
		t.Fatal(err)
	}
	if thirdResult.Filename != "runner-1-glyphflow-runner-linux-amd64-headless" {
		t.Fatalf("headless filename = %q", thirdResult.Filename)
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
	disabled := filterRunners([]RunnerRecord{
		{ID: "runner-1", DesiredState: "ENABLED"},
		{ID: "runner-2", DesiredState: "DISABLED"},
	}, "", "", "DISABLED")
	if len(disabled) != 1 || disabled[0].ID != "runner-2" {
		t.Fatalf("disabled runners = %#v", disabled)
	}
}

func TestRunnerRevokeCanBeReset(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1", DesiredState: "DISABLED", ObservedState: "REVOKED"}
	response := httptest.NewRecorder()
	s.runnerPath(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/runner-1/reset", nil))
	if response.Code != http.StatusOK || s.runners["runner-1"].DesiredState != "ENABLED" || s.runners["runner-1"].ObservedState != "OFFLINE" {
		t.Fatalf("reset returned %d and runner = %#v", response.Code, s.runners["runner-1"])
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

func TestFilterResourcesByKind(t *testing.T) {
	items := filterResources([]ResourceRecord{
		{ID: "exclusive", Kind: "exclusive"},
		{ID: "non-blocking", Kind: "non_blocking"},
	}, "", "exclusive")
	if len(items) != 1 || items[0].ID != "exclusive" {
		t.Fatalf("exclusive resources = %#v", items)
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
	response = httptest.NewRecorder()
	s.resourceCollection(response, httptest.NewRequest(http.MethodPost, "/api/v1/resources", bytes.NewBufferString(`{"name":"Telemetry","kind":"non-blocking"}`)))
	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte(`"kind":"non-blocking"`)) {
		t.Fatalf("create non-blocking resource returned %d: %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.resourceCollection(response, httptest.NewRequest(http.MethodPost, "/api/v1/resources", bytes.NewBufferString(`{"name":"Invalid","kind":"shared"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid resource kind returned %d", response.Code)
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

func TestRunnerMetricsHistoryHonorsRangeAndLimit(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.runnerMetrics["runner-1"] = []store.RunnerMetricsRecord{
		{SampledAt: base, CPUPercent: 10, MemoryPercent: 20, MemoryTotalBytes: 100},
		{SampledAt: base.Add(time.Hour), CPUPercent: 30, MemoryPercent: 40, MemoryTotalBytes: 100},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runners/runner-1/metrics?from=2026-01-01T00:30:00Z&to=2026-01-01T02:00:00Z&limit=1", nil)
	response := httptest.NewRecorder()
	s.runnerMetricsPath(response, request, "runner-1")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cpuPercent":30`) || strings.Contains(response.Body.String(), `"cpuPercent":10`) {
		t.Fatalf("metrics response = %d: %s", response.Code, response.Body.String())
	}
}

func TestRunnerListingIncludesLatestResourceMetrics(t *testing.T) {
	s := NewInfrastructureService()
	s.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "runner-1"}
	s.runnerMetrics["runner-1"] = []store.RunnerMetricsRecord{{SampledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), CPUPercent: 22.5, MemoryPercent: 45}}
	response := httptest.NewRecorder()
	s.runnerCollection(response, httptest.NewRequest(http.MethodGet, "/api/v1/runners", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cpuPercent":22.5`) || !strings.Contains(response.Body.String(), `"memoryPercent":45`) {
		t.Fatalf("runner listing = %d: %s", response.Code, response.Body.String())
	}
}

func TestRunnerMetricsHistoryRejectsUnknownRunnerAndInvalidRange(t *testing.T) {
	for path, want := range map[string]int{
		"/api/v1/runners/missing/metrics":          http.StatusNotFound,
		"/api/v1/runners/missing/metrics?from=bad": http.StatusBadRequest,
	} {
		s := NewInfrastructureService()
		response := httptest.NewRecorder()
		s.runnerMetricsPath(response, httptest.NewRequest(http.MethodGet, path, nil), "missing")
		if response.Code != want {
			t.Fatalf("%s returned %d, want %d", path, response.Code, want)
		}
	}
}
