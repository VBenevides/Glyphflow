package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type DeadLetterView struct {
	ID            string    `json:"id"`
	RunnerID      string    `json:"runnerId"`
	Stream        string    `json:"stream"`
	Consumer      string    `json:"consumer"`
	Subject       string    `json:"subject"`
	MessageID     string    `json:"messageId"`
	PayloadSHA256 string    `json:"payloadSha256"`
	Error         string    `json:"error,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	State         string    `json:"state"`
	Attempts      uint64    `json:"attempts"`
	FirstFailedAt time.Time `json:"firstFailedAt"`
	LastFailedAt  time.Time `json:"lastFailedAt"`
}

const deadLetterDiagnosticLimit = 256

func safeDeadLetterDiagnostic(value string) string {
	value = redactSensitiveText(value)
	runes := []rune(value)
	if len(runes) > deadLetterDiagnosticLimit {
		return string(runes[:deadLetterDiagnosticLimit])
	}
	return value
}

type DeadLetterService struct {
	repository store.DeadLetterRepository
	publisher  queue.Publisher
}

func NewDeadLetterService(repository store.DeadLetterRepository, publisher queue.Publisher) *DeadLetterService {
	return &DeadLetterService{repository: repository, publisher: publisher}
}

func (s *DeadLetterService) SetRepository(repository store.DeadLetterRepository) {
	if repository != nil {
		s.repository = repository
	}
}

func (s *DeadLetterService) SetPublisher(publisher queue.Publisher) {
	if publisher != nil {
		s.publisher = publisher
	}
}

func deadLetterView(item store.DeadLetterSummary) DeadLetterView {
	return DeadLetterView{ID: item.ID, RunnerID: item.RunnerID, Stream: item.Stream, Consumer: item.Consumer, Subject: item.Subject, MessageID: item.MessageID, PayloadSHA256: item.PayloadSHA256, Error: safeDeadLetterDiagnostic(item.Error), CorrelationID: item.CorrelationID, State: item.State, Attempts: item.Attempts, FirstFailedAt: item.FirstFailedAt.UTC(), LastFailedAt: item.LastFailedAt.UTC()}
}

func (s *DeadLetterService) collection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	state := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("state")))
	if state != "" && state != "OPEN" && state != "RETRY_QUEUED" && state != "RECONCILED" && state != "DISCARDED" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid dead-letter state"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if _, ok := checkedPaginationOffset(page, limit); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": paginationOffsetError})
		return
	}
	if s == nil || s.repository == nil {
		writeError(w, http.StatusServiceUnavailable, "dead-letter storage unavailable", nil)
		return
	}
	items, total, err := s.repository.List(r.Context(), store.DeadLetterFilter{State: state, RunnerID: strings.TrimSpace(r.URL.Query().Get("runnerId")), Subject: strings.TrimSpace(r.URL.Query().Get("subject")), Page: page, Limit: limit})
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter storage unavailable", err)
		return
	}
	views := make([]DeadLetterView, 0, len(items))
	for _, item := range items {
		views = append(views, deadLetterView(item))
	}
	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views, "page": page, "limit": limit, "total": total, "pages": pages})
}

func (s *DeadLetterService) path(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/dead-letters/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "dead letter not found"})
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		s.detail(w, r, id)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	switch parts[1] {
	case "retry":
		s.retry(w, r, id)
	case "reconcile":
		s.reconcile(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "dead-letter action not found"})
	}
}

func (s *DeadLetterService) detail(w http.ResponseWriter, r *http.Request, id string) {
	if s == nil || s.repository == nil {
		writeError(w, http.StatusServiceUnavailable, "dead-letter storage unavailable", nil)
		return
	}
	item, found, err := s.repository.Find(r.Context(), id)
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter storage unavailable", err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "dead letter not found"})
		return
	}
	writeJSON(w, http.StatusOK, deadLetterView(item))
}

type deadLetterActionInput struct {
	Reason string `json:"reason"`
	State  string `json:"state"`
}

func decodeDeadLetterAction(r *http.Request) (deadLetterActionInput, error) {
	var input deadLetterActionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		return input, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || len(input.Reason) > 512 {
		return input, errInvalidDeadLetterReason
	}
	input.Reason = safeDeadLetterDiagnostic(input.Reason)
	return input, nil
}

var errInvalidDeadLetterReason = &deadLetterActionError{"reason is required and must be at most 512 bytes"}

type deadLetterActionError struct{ message string }

func (e *deadLetterActionError) Error() string { return e.message }

func (s *DeadLetterService) retry(w http.ResponseWriter, r *http.Request, id string) {
	if s == nil || s.repository == nil || s.publisher == nil {
		writeError(w, http.StatusServiceUnavailable, "dead-letter recovery unavailable", nil)
		return
	}
	recordRequestAuditField(r, "deadLetterId", id)
	input, err := decodeDeadLetterAction(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	retry, claimed, err := s.repository.BeginRetry(r.Context(), id)
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter recovery unavailable", err)
		return
	}
	if !claimed {
		if retry.MessageID != "" {
			recordRequestAuditField(r, "messageId", retry.MessageID)
		}
		s.writeTransitionConflict(w, r, id)
		return
	}
	recordRequestAuditField(r, "messageId", retry.MessageID)
	deliveryID, err := randomID()
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter recovery unavailable", err)
		return
	}
	if err := s.publisher.Publish(r.Context(), queue.Message{Subject: retry.Subject, Data: retry.Payload, ID: "dead-letter-retry-" + deliveryID}); err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter retry could not be published", err)
		return
	}
	recordRequestAuditField(r, "deliveryId", "dead-letter-retry-"+deliveryID)
	writeJSON(w, http.StatusAccepted, map[string]any{"id": retry.ID, "messageId": retry.MessageID, "deliveryId": "dead-letter-retry-" + deliveryID, "state": "RETRY_QUEUED", "reason": input.Reason})
}

func (s *DeadLetterService) reconcile(w http.ResponseWriter, r *http.Request, id string) {
	if s == nil || s.repository == nil {
		writeError(w, http.StatusServiceUnavailable, "dead-letter storage unavailable", nil)
		return
	}
	recordRequestAuditField(r, "deadLetterId", id)
	input, err := decodeDeadLetterAction(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	input.State = strings.ToUpper(strings.TrimSpace(input.State))
	if input.State != "RECONCILED" && input.State != "DISCARDED" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state must be RECONCILED or DISCARDED"})
		return
	}
	item, found, err := s.repository.Find(r.Context(), id)
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter reconciliation unavailable", err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "dead letter not found"})
		return
	}
	recordRequestAuditField(r, "messageId", item.MessageID)
	changed, err := s.repository.Reconcile(r.Context(), id, input.State)
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "dead-letter reconciliation unavailable", err)
		return
	}
	if !changed {
		s.writeTransitionConflict(w, r, id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "state": input.State, "reason": input.Reason})
}

func (s *DeadLetterService) writeTransitionConflict(w http.ResponseWriter, r *http.Request, id string) {
	if s.repository != nil {
		if item, found, err := s.repository.Find(r.Context(), id); err == nil && !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "dead letter not found"})
			return
		} else if err != nil {
			recordRequestError(r, err)
			writeError(w, http.StatusServiceUnavailable, "dead-letter storage unavailable", err)
			return
		} else if found {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "dead letter is not actionable", "state": item.State})
			return
		}
	}
	writeJSON(w, http.StatusConflict, map[string]string{"error": "dead letter transition was not applied"})
}
