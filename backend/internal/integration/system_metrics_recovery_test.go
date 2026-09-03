//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/api"
	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
)

func TestSystemMetricsDeadLetterRecoveryGate(t *testing.T) { // NOSONAR: this integration gate intentionally covers dead-letter persistence, API inspection, retry, reconciliation, audit, and alert recovery together.
	databaseURL, natsURL := os.Getenv("DATABASE_URL"), os.Getenv("NATS_TLS_URL")
	if databaseURL == "" || natsURL == "" || os.Getenv("NATS_CERT_FILE") == "" || os.Getenv("NATS_KEY_FILE") == "" || os.Getenv("NATS_CA_FILE") == "" {
		t.Skip("set database and mutual-TLS NATS variables to run the operational recovery gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.ApplyMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	stream, err := queue.ConnectJetStreamTLS(natsURL, queue.TLSConfig{CertificateFile: os.Getenv("NATS_CERT_FILE"), KeyFile: os.Getenv("NATS_KEY_FILE"), CAFile: os.Getenv("NATS_CA_FILE")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	subject := queue.Subject("events", "gate-runner-"+suffix)
	durable := "gate-consumer-" + suffix
	repository := store.NewDeadLetterRepository(db, []byte("01234567890123456789012345678901"))
	stream.SetDeadLetterSink(func(ctx context.Context, record queue.DeadLetter) error {
		return repository.Persist(ctx, store.DeadLetterRecord{RunnerID: record.RunnerID, Stream: record.Stream, Consumer: record.Consumer, Subject: record.Subject, MessageID: record.MessageID, Payload: record.Payload, Error: record.Error, Attempts: record.Attempts, FirstFailedAt: record.FirstFailedAt, LastFailedAt: record.LastFailedAt, CorrelationID: record.CorrelationID})
	})
	consumer, err := stream.Consumer(ctx, durable, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM dead_letters WHERE stream = $1 AND consumer = $2`, "GLYPHFLOW", durable)
	})

	originalSignatureID := "gate-signature-" + suffix
	originalAttemptID := "gate-attempt-" + suffix
	if err := stream.Publish(ctx, queue.Message{Subject: subject, ID: originalSignatureID, Data: []byte("signature-payload")}); err != nil {
		t.Fatal(err)
	}
	exhaustDeadLetter(t, ctx, stream, consumer, originalSignatureID, errors.New("signature rejected: unknown key"))
	if err := stream.Publish(ctx, queue.Message{Subject: subject, ID: originalAttemptID, Data: []byte("unknown-attempt-payload")}); err != nil {
		t.Fatal(err)
	}
	exhaustDeadLetter(t, ctx, stream, consumer, originalAttemptID, errors.New("unknown attempt"))

	items, total, err := repository.List(ctx, store.DeadLetterFilter{Subject: subject, State: "OPEN", Page: 1, Limit: 10})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("dead-letter records = %#v total=%d err=%v", items, total, err)
	}
	for _, item := range items {
		if item.MessageID != originalSignatureID && item.MessageID != originalAttemptID {
			t.Fatalf("original message identity was not preserved: %#v", item)
		}
	}
	retryID, reconciledID := "", ""
	for _, item := range items {
		if item.MessageID == originalSignatureID {
			retryID = item.ID
		} else {
			reconciledID = item.ID
		}
	}
	if retryID == "" || reconciledID == "" {
		t.Fatalf("dead-letter IDs were not classified: retry=%q reconcile=%q", retryID, reconciledID)
	}

	audit := api.NewAuditQueryService()
	audit.SetRepository(store.NewAuditRepository(db))
	metrics := new(platform.Metrics)
	server := api.Server{
		Auth: func(*http.Request) (api.Claims, bool) { return api.Claims{}, true },
		Permissions: func(api.Claims) map[string]bool {
			return map[string]bool{"system.metrics.read": true, "system.deadletter.read": true, "system.deadletter.manage": true}
		},
		Metrics: metrics,
		SystemMetrics: &api.SystemMetricsService{Metrics: metrics, Ready: func(context.Context) error { return nil }, Signals: func(ctx context.Context) (platform.OperationalSignals, error) {
			stats, err := repository.Stats(ctx)
			return platform.OperationalSignals{DeadLetters: platform.DeadLetterSignals{Open: stats.Open, OldestAgeSeconds: stats.OldestAgeSeconds}}, err
		}},
		DeadLetters: api.NewDeadLetterService(repository, stream),
		AuditQuery:  audit,
	}
	handler := server.Handler()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/metrics", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"deadLetters"`) {
		t.Fatalf("metrics response = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	listURL := "/api/v1/admin/dead-letters?subject=" + url.QueryEscape(subject) + "&state=OPEN&limit=10"
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, listURL, nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "signature-payload") || strings.Contains(response.Body.String(), "unknown-attempt-payload") {
		t.Fatalf("safe dead-letter inspection = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	retryPath := "/api/v1/admin/dead-letters/" + url.PathEscape(retryID) + "/retry"
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, retryPath, strings.NewReader(`{"reason":"replay after operator review"}`)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry response = %d %s", response.Code, response.Body.String())
	}
	var retryResponse struct {
		DeliveryID string `json:"deliveryId"`
		MessageID  string `json:"messageId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &retryResponse); err != nil || retryResponse.MessageID == "" || retryResponse.DeliveryID == "" || retryResponse.MessageID == retryResponse.DeliveryID {
		t.Fatalf("retry identity = %#v err=%v", retryResponse, err)
	}
	var retried queue.Message
	if err := stream.ConsumeOne(ctx, consumer, func(_ context.Context, message queue.Message) error { retried = message; return nil }); err != nil {
		t.Fatal(err)
	}
	if retried.ID != retryResponse.DeliveryID || string(retried.Data) != "signature-payload" {
		t.Fatalf("replayed message = %#v", retried)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, retryPath, strings.NewReader(`{"reason":"duplicate operator request"}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate retry response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	reconcilePath := "/api/v1/admin/dead-letters/" + url.PathEscape(reconciledID) + "/reconcile"
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, reconcilePath, strings.NewReader(`{"state":"DISCARDED","reason":"confirmed unknown attempt"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("reconcile response = %d %s", response.Code, response.Body.String())
	}
	if item, found, err := store.NewDeadLetterRepository(db, []byte("01234567890123456789012345678901")).Find(ctx, reconciledID); err != nil || !found || item.State != "DISCARDED" {
		t.Fatalf("restart-safe terminal state = %#v found=%t err=%v", item, found, err)
	}
	if events, _, err := store.NewAuditRepository(db).Query(ctx, store.AuditFilter{Target: retryPath, Page: 1, Limit: 10}); err != nil || len(events) == 0 {
		t.Fatalf("retry audit events = %#v err=%v", events, err)
	}
	alerts := platform.EvaluateOperationalAlerts(platform.OperationalSignals{DeadLetters: platform.DeadLetterSignals{Open: 2}}, platform.DefaultAlertThresholds())
	if len(alerts) == 0 || alerts[0].Code != "dead_letters_open" || alerts[0].Status != "firing" {
		t.Fatalf("dead-letter alert = %#v", alerts)
	}
	resolved := platform.EvaluateOperationalAlerts(platform.OperationalSignals{}, platform.DefaultAlertThresholds())
	if len(resolved) == 0 || resolved[0].Code != "dead_letters_open" || resolved[0].Status != "resolved" {
		t.Fatalf("resolved dead-letter alert = %#v", resolved)
	}
}

func exhaustDeadLetter(t *testing.T, ctx context.Context, stream *queue.JetStream, consumer jetstream.Consumer, messageID string, failure error) {
	t.Helper()
	for range 5 {
		if err := stream.ConsumeOne(ctx, consumer, func(_ context.Context, message queue.Message) error {
			if message.ID != messageID {
				t.Fatalf("unexpected message %q while exhausting %q", message.ID, messageID)
			}
			return failure
		}); err != nil {
			t.Fatal(err)
		}
	}
}
