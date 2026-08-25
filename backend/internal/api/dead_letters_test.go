package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type deadLetterRepositoryStub struct {
	item       store.DeadLetterSummary
	state      string
	beginCalls int
}

func (s *deadLetterRepositoryStub) Persist(context.Context, store.DeadLetterRecord) error { return nil }
func (s *deadLetterRepositoryStub) List(context.Context, store.DeadLetterFilter) ([]store.DeadLetterSummary, int, error) {
	if s.state == "" {
		return nil, 0, nil
	}
	s.item.State = s.state
	return []store.DeadLetterSummary{s.item}, 1, nil
}
func (s *deadLetterRepositoryStub) Find(context.Context, string) (store.DeadLetterSummary, bool, error) {
	if s.state == "" {
		return store.DeadLetterSummary{}, false, nil
	}
	s.item.State = s.state
	return s.item, true, nil
}
func (s *deadLetterRepositoryStub) BeginRetry(context.Context, string) (store.DeadLetterRetry, bool, error) {
	s.beginCalls++
	if s.state != "OPEN" {
		return store.DeadLetterRetry{ID: s.item.ID, Subject: s.item.Subject, MessageID: s.item.MessageID}, false, nil
	}
	s.state = "RETRY_QUEUED"
	return store.DeadLetterRetry{ID: s.item.ID, Subject: s.item.Subject, MessageID: s.item.MessageID, Payload: []byte("exact-payload")}, true, nil
}
func (s *deadLetterRepositoryStub) Reconcile(_ context.Context, _ string, state string) (bool, error) {
	if s.state != "OPEN" && s.state != "RETRY_QUEUED" {
		return false, nil
	}
	s.state = state
	return true, nil
}
func (s *deadLetterRepositoryStub) Stats(context.Context) (store.DeadLetterStats, error) {
	if s.state == "OPEN" {
		return store.DeadLetterStats{Open: 1, OldestAgeSeconds: 10}, nil
	}
	return store.DeadLetterStats{}, nil
}

type deadLetterPublisherStub struct {
	message queue.Message
	count   int
}

func (p *deadLetterPublisherStub) Publish(_ context.Context, message queue.Message) error {
	p.message, p.count = message, p.count+1
	return nil
}

func TestDeadLetterRecoveryUsesCASAndNewDeliveryIdentity(t *testing.T) {
	repository := &deadLetterRepositoryStub{state: "OPEN", item: store.DeadLetterSummary{ID: "dead-1", Stream: "GLYPHFLOW", Consumer: "control-plane", Subject: "glyphflow.events.runner-1", MessageID: "event-1", Error: "signature rejected", Attempts: 5, FirstFailedAt: time.Now().UTC(), LastFailedAt: time.Now().UTC()}}
	publisher := &deadLetterPublisherStub{}
	server := Server{
		Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "operator"}, true },
		Permissions: func(Claims) map[string]bool {
			return map[string]bool{"system.deadletter.read": true, "system.deadletter.manage": true}
		},
		DeadLetters: NewDeadLetterService(repository, publisher),
	}
	handler := server.Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/dead-letters/dead-1", nil))
	if response.Code != http.StatusOK || string(response.Body.Bytes()) == "" {
		t.Fatalf("detail status = %d, body = %s", response.Code, response.Body.String())
	}
	if string(response.Body.Bytes()) == "exact-payload" {
		t.Fatal("dead-letter detail exposed the payload")
	}

	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-1/retry", strings.NewReader(`{"reason":"replay after key rotation"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, body = %s", response.Code, response.Body.String())
	}
	var retry map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &retry); err != nil {
		t.Fatal(err)
	}
	if retry["messageId"] != "event-1" || retry["state"] != "RETRY_QUEUED" || publisher.message.ID == "event-1" || publisher.message.Subject != repository.item.Subject || string(publisher.message.Data) != "exact-payload" {
		t.Fatalf("retry identity = %#v, published = %#v", retry, publisher.message)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-1/retry", strings.NewReader(`{"reason":"repeat"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || publisher.count != 1 {
		t.Fatalf("duplicate retry status = %d, publishes = %d", response.Code, publisher.count)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-1/reconcile", strings.NewReader(`{"state":"DISCARDED","reason":"confirmed invalid signature"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repository.state != "DISCARDED" {
		t.Fatalf("reconcile status = %d, state = %s", response.Code, repository.state)
	}
}

func TestDeadLetterReadPermissionCannotMutate(t *testing.T) {
	server := Server{
		Auth:        func(*http.Request) (Claims, bool) { return Claims{}, true },
		Permissions: func(Claims) map[string]bool { return map[string]bool{"system.deadletter.read": true} },
		DeadLetters: NewDeadLetterService(&deadLetterRepositoryStub{state: "OPEN"}, &deadLetterPublisherStub{}),
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-1/retry", strings.NewReader(`{"reason":"operator review"}`)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("read-only retry status = %d", response.Code)
	}
}

func TestPermissionDenialIsCountedAndAuditedWithoutRequestBody(t *testing.T) {
	audit := NewAuditQueryService()
	metrics := new(platform.Metrics)
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "operator"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"system.deadletter.read": true} }, Metrics: metrics, AuditQuery: audit, DeadLetters: NewDeadLetterService(&deadLetterRepositoryStub{state: "OPEN"}, &deadLetterPublisherStub{})}
	response := server.Handler()
	recorded := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-1/retry", strings.NewReader(`{"reason":"do not store this payload"}`))
	response.ServeHTTP(recorded, req)
	if recorded.Code != http.StatusForbidden || metrics.PermissionDenials.Load() != 1 || len(audit.events) != 1 {
		t.Fatalf("denial status=%d metric=%d audit=%d", recorded.Code, metrics.PermissionDenials.Load(), len(audit.events))
	}
	if audit.events[0].Input != nil || audit.events[0].Target != "/api/v1/admin/dead-letters/dead-1/retry" {
		t.Fatalf("denial audit exposed input: %#v", audit.events[0])
	}
}
