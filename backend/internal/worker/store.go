package worker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	_ "modernc.org/sqlite"
)

type LocalStore struct{ db *sql.DB }

type InboxOrder struct {
	OrderID, ExecutionAttemptID, RunID, TaskVersionID, RunnerID, RunnerSessionID, ExecutorBootID, Envelope, State, LeaseToken, ExecutionSpecDigest string
	FencingToken                                                                                                                                   int64
	LeaseNotAfter                                                                                                                                  time.Time
	ProcessID                                                                                                                                      int64
}
type OutboxEvent struct {
	EventID, OrderID, Channel, EventType, Envelope, State string
	Sequence                                              int64
	PublishedAt                                           *time.Time
}

func OpenStore(path string) (*LocalStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &LocalStore{db: db}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON; CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, value BLOB NOT NULL); CREATE TABLE IF NOT EXISTS order_inbox (order_id TEXT PRIMARY KEY, execution_attempt_id TEXT NOT NULL UNIQUE, run_id TEXT NOT NULL, task_version_id TEXT NOT NULL, target_runner_id TEXT NOT NULL, target_runner_session_id TEXT NOT NULL, executor_boot_id TEXT, envelope BLOB NOT NULL, state TEXT NOT NULL, lease_token TEXT NOT NULL, fencing_token INTEGER NOT NULL, lease_not_after TEXT NOT NULL, execution_spec_digest TEXT NOT NULL, process_id INTEGER, received_at TEXT NOT NULL, claimed_at TEXT, process_started_at TEXT, finished_at TEXT, last_error TEXT); CREATE TABLE IF NOT EXISTS event_outbox (event_id TEXT PRIMARY KEY, order_id TEXT NOT NULL, event_channel TEXT NOT NULL, channel_sequence INTEGER NOT NULL, event_type TEXT NOT NULL, envelope BLOB NOT NULL, state TEXT NOT NULL, publish_attempts INTEGER NOT NULL DEFAULT 0, available_at TEXT NOT NULL, published_at TEXT, last_error TEXT, created_at TEXT NOT NULL, UNIQUE(order_id, event_channel, channel_sequence));`); err != nil {
		_ = db.Close()
		return nil, err
	}
	var journal, synchronous, foreignKeys string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		_ = db.Close()
		return nil, errors.New("SQLite WAL mode is not active")
	}
	if err := db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil || synchronous != "2" {
		_ = db.Close()
		return nil, errors.New("SQLite FULL synchronization is not active")
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != "1" {
		_ = db.Close()
		return nil, errors.New("SQLite foreign keys are not active")
	}
	return store, nil
}

func (s *LocalStore) PutOrder(order InboxOrder) error {
	if order.OrderID == "" || order.ExecutionAttemptID == "" || order.RunID == "" || order.RunnerID == "" || order.RunnerSessionID == "" || order.Envelope == "" {
		return errors.New("order inbox fields are required")
	}
	_, err := s.db.Exec(`INSERT INTO order_inbox (order_id, execution_attempt_id, run_id, task_version_id, target_runner_id, target_runner_session_id, executor_boot_id, envelope, state, lease_token, fencing_token, lease_not_after, execution_spec_digest, process_id, received_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'RECEIVED', ?, ?, ?, ?, ?, ?) ON CONFLICT(order_id) DO NOTHING`, order.OrderID, order.ExecutionAttemptID, order.RunID, order.TaskVersionID, order.RunnerID, order.RunnerSessionID, order.ExecutorBootID, []byte(order.Envelope), order.LeaseToken, order.FencingToken, order.LeaseNotAfter.UTC().Format(time.RFC3339Nano), order.ExecutionSpecDigest, order.ProcessID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *LocalStore) ClaimOrder(orderID, bootID string, processID int64) error {
	result, err := s.db.Exec(`UPDATE order_inbox SET state='CLAIMED', executor_boot_id=?, process_id=?, claimed_at=? WHERE order_id=? AND state='RECEIVED'`, bootID, processID, time.Now().UTC().Format(time.RFC3339Nano), orderID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("order is not claimable")
	}
	return nil
}
func (s *LocalStore) MarkProcessStarted(orderID string) error {
	_, err := s.db.Exec(`UPDATE order_inbox SET state='RUNNING', process_started_at=? WHERE order_id=? AND state='CLAIMED'`, time.Now().UTC().Format(time.RFC3339Nano), orderID)
	return err
}
func (s *LocalStore) FinishOrder(orderID, state, lastError string) error {
	_, err := s.db.Exec(`UPDATE order_inbox SET state=?, last_error=?, finished_at=? WHERE order_id=? AND state IN ('CLAIMED','RUNNING')`, state, lastError, time.Now().UTC().Format(time.RFC3339Nano), orderID)
	return err
}
func (s *LocalStore) RecoverOrders(previousBootID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT order_id FROM order_inbox WHERE executor_boot_id=? AND state IN ('CLAIMED','RUNNING')`, previousBootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE order_inbox SET state='UNKNOWN', last_error='runner restart' WHERE executor_boot_id=? AND state IN ('CLAIMED','RUNNING')`, previousBootID); err != nil {
		return nil, err
	}
	return ids, nil
}
func (s *LocalStore) PutEvent(event OutboxEvent) error {
	if event.EventID == "" || event.OrderID == "" || event.Channel == "" || event.Sequence <= 0 || event.Envelope == "" {
		return errors.New("event outbox fields are required")
	}
	_, err := s.db.Exec(`INSERT INTO event_outbox (event_id, order_id, event_channel, channel_sequence, event_type, envelope, state, available_at, created_at) VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?, ?) ON CONFLICT(event_id) DO NOTHING`, event.EventID, event.OrderID, event.Channel, event.Sequence, event.EventType, []byte(event.Envelope), time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *LocalStore) PendingEvents(limit int) ([]OutboxEvent, error) {
	rows, err := s.db.Query(`SELECT event_id, order_id, event_channel, channel_sequence, event_type, envelope, state FROM event_outbox WHERE state='PENDING' ORDER BY created_at LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		var envelope []byte
		if err := rows.Scan(&event.EventID, &event.OrderID, &event.Channel, &event.Sequence, &event.EventType, &envelope, &event.State); err != nil {
			return nil, err
		}
		event.Envelope = string(envelope)
		events = append(events, event)
	}
	return events, rows.Err()
}
func (s *LocalStore) MarkEventPublished(eventID string) error {
	_, err := s.db.Exec(`UPDATE event_outbox SET state='PUBLISHED', published_at=? WHERE event_id=? AND state='PENDING'`, time.Now().UTC().Format(time.RFC3339Nano), eventID)
	return err
}

func (s *LocalStore) Close() error { return s.db.Close() }

func (s *LocalStore) Put(id string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO messages (id, value) VALUES (?, ?) ON CONFLICT(id) DO NOTHING`, id, raw)
	return err
}

func (s *LocalStore) Get(id string) (json.RawMessage, error) {
	var raw []byte
	if err := s.db.QueryRow(`SELECT value FROM messages WHERE id = ?`, id).Scan(&raw); err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// AcceptOrder verifies freshness, identity, execution fields, and replay
// before writing the order to durable local state.
func (s *LocalStore) AcceptOrder(raw []byte, keys protocol.Keyring, now time.Time, runnerID, runID string, attempt uint32, leaseToken string, tolerance time.Duration) (protocol.OrderPayload, error) {
	payload, err := protocol.VerifyOrder(raw, keys, now, runnerID, runID, attempt, leaseToken, tolerance, nil)
	if err != nil {
		return protocol.OrderPayload{}, err
	}
	if err := s.Put(payload.OrderID, payload); err != nil {
		return protocol.OrderPayload{}, err
	}
	if err := s.PutOrder(InboxOrder{OrderID: payload.OrderID, ExecutionAttemptID: payload.OrderID + "-attempt", RunID: payload.RunID, TaskVersionID: payload.TaskID, RunnerID: payload.RunnerID, RunnerSessionID: "legacy", Envelope: string(raw), State: "RECEIVED", LeaseToken: payload.LeaseToken, LeaseNotAfter: payload.ExpiresAt}); err != nil {
		return protocol.OrderPayload{}, err
	}
	return payload, nil
}
