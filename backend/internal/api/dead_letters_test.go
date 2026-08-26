package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	deliveryID string
	beginCalls int
	listCalls  int
	lastFilter store.DeadLetterFilter
}

func (s *deadLetterRepositoryStub) Persist(context.Context, store.DeadLetterRecord) error { return nil }
func (s *deadLetterRepositoryStub) List(_ context.Context, filter store.DeadLetterFilter) ([]store.DeadLetterSummary, int, error) {
	s.listCalls++
	s.lastFilter = filter
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
	return store.DeadLetterRetry{ID: s.item.ID, Subject: s.item.Subject, MessageID: s.item.MessageID, Payload: []byte("exact-payload"), DeliveryID: s.deliveryID, Attempts: 1}, true, nil
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

func TestDeadLetterRecoveryUsesPersistedDeliveryIdentity(t *testing.T) {
	repository := &deadLetterRepositoryStub{state: "OPEN", deliveryID: "delivery-1", item: store.DeadLetterSummary{ID: "dead-persisted", Subject: "glyphflow.events.runner-1", MessageID: "event-persisted", FirstFailedAt: time.Now().UTC(), LastFailedAt: time.Now().UTC()}}
	publisher := &deadLetterPublisherStub{}
	server := NewDeadLetterService(repository, publisher)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-persisted/retry", strings.NewReader(`{"reason":"operator review"}`))
	response := httptest.NewRecorder()
	server.retry(response, request, "dead-persisted")
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, body = %s", response.Code, response.Body.String())
	}
	if publisher.message.ID != "dead-letter-retry-delivery-1" {
		t.Fatalf("delivery ID = %q", publisher.message.ID)
	}
}

func TestDeadLetterInspectionRedactsSensitiveDiagnostics(t *testing.T) {
	repository := &deadLetterRepositoryStub{state: "OPEN", item: store.DeadLetterSummary{ID: "dead-sensitive", Subject: "glyphflow.events.runner-1", MessageID: "event-sensitive", Error: "password=top-secret token=abc", FirstFailedAt: time.Now().UTC(), LastFailedAt: time.Now().UTC()}}
	server := Server{
		Auth:        func(*http.Request) (Claims, bool) { return Claims{UserID: "operator"}, true },
		Permissions: func(Claims) map[string]bool { return map[string]bool{"system.deadletter.read": true} },
		DeadLetters: NewDeadLetterService(repository, nil),
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/dead-letters/dead-sensitive", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "[REDACTED]") || strings.Contains(body, "top-secret") || strings.Contains(body, "abc") {
		t.Fatalf("sensitive diagnostic response = %d %s", response.Code, body)
	}
}

func TestDeadLetterRecoveryAuditContainsIdentityAndRedactsReason(t *testing.T) {
	repository := &deadLetterRepositoryStub{state: "OPEN", item: store.DeadLetterSummary{ID: "dead-audit", Subject: "glyphflow.events.runner-1", MessageID: "event-audit", FirstFailedAt: time.Now().UTC(), LastFailedAt: time.Now().UTC()}}
	audit := NewAuditQueryService()
	server := Server{
		Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "operator"}, true },
		Permissions: func(Claims) map[string]bool {
			return map[string]bool{"system.deadletter.read": true, "system.deadletter.manage": true}
		},
		AuditQuery:  audit,
		DeadLetters: NewDeadLetterService(repository, &deadLetterPublisherStub{}),
	}
	handler := server.Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-audit/retry", strings.NewReader(`{"reason":"replay after review"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, body = %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-audit/retry", strings.NewReader(`{"reason":"token=do-not-store"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate retry status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/dead-letters/dead-audit/reconcile", strings.NewReader(`{"state":"DISCARDED","reason":"confirmed invalid signature"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("reconcile status = %d, body = %s", response.Code, response.Body.String())
	}
	var retryAudit, deniedRetryAudit, reconcileAudit *AuditEvent
	for index := range audit.events {
		event := &audit.events[index]
		switch event.Target {
		case "/api/v1/admin/dead-letters/dead-audit/retry":
			if retryAudit == nil {
				retryAudit = event
			} else {
				deniedRetryAudit = event
			}
		case "/api/v1/admin/dead-letters/dead-audit/reconcile":
			reconcileAudit = event
		}
	}
	if retryAudit == nil || reconcileAudit == nil || retryAudit.Actor != "operator" || retryAudit.Result != "success" || reconcileAudit.Result != "success" {
		t.Fatalf("recovery audit events = %#v", audit.events)
	}
	retryInput := retryAudit.Input.(map[string]any)
	if retryInput["deadLetterId"] != "dead-audit" || retryInput["messageId"] != "event-audit" || retryInput["deliveryId"] == nil || retryInput["body"].(map[string]any)["reason"] != "replay after review" {
		t.Fatalf("retry audit input = %#v", retryInput)
	}
	reconcileInput := reconcileAudit.Input.(map[string]any)
	if reconcileInput["deadLetterId"] != "dead-audit" || reconcileInput["messageId"] != "event-audit" {
		t.Fatalf("reconcile audit input = %#v", reconcileInput)
	}
	if deniedRetryAudit == nil || deniedRetryAudit.Input.(map[string]any)["body"].(map[string]any)["reason"] != "[REDACTED]" {
		t.Fatalf("sensitive retry reason audit = %#v", deniedRetryAudit)
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

func TestDeadLetterCollectionPreservesNormalAndClampedPagination(t *testing.T) {
	for _, test := range []struct {
		name        string
		path        string
		page, limit int
	}{
		{name: "normal", path: "/api/v1/admin/dead-letters?page=2&limit=10", page: 2, limit: 10},
		{name: "clamped", path: "/api/v1/admin/dead-letters?page=0&limit=1000", page: 1, limit: 50},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &deadLetterRepositoryStub{state: "OPEN"}
			service := NewDeadLetterService(repository, nil)
			response := httptest.NewRecorder()
			service.collection(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK || repository.listCalls != 1 {
				t.Fatalf("status=%d list calls=%d body=%s", response.Code, repository.listCalls, response.Body.String())
			}
			if repository.lastFilter.Page != test.page || repository.lastFilter.Limit != test.limit {
				t.Fatalf("filter=%#v, want page=%d limit=%d", repository.lastFilter, test.page, test.limit)
			}
		})
	}
}

func TestDeadLetterCollectionRejectsOverflowingPaginationOffset(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, page := range []int{maxInt/100 + 2, maxInt} {
		repository := &deadLetterRepositoryStub{state: "OPEN"}
		service := NewDeadLetterService(repository, nil)
		response := httptest.NewRecorder()
		path := "/api/v1/admin/dead-letters?page=" + strconv.Itoa(page) + "&limit=100"
		service.collection(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), paginationOffsetError) {
			t.Fatalf("page=%d response=%d body=%s", page, response.Code, response.Body.String())
		}
		if repository.listCalls != 0 {
			t.Fatalf("page=%d reached the store", page)
		}
	}
}
