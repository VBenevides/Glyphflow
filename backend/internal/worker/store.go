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

func OpenStore(path string) (*LocalStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &LocalStore{db: db}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON; CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, value BLOB NOT NULL); CREATE TABLE IF NOT EXISTS order_inbox (order_id TEXT PRIMARY KEY, execution_attempt_id TEXT NOT NULL UNIQUE, envelope BLOB NOT NULL, state TEXT NOT NULL, received_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS event_outbox (event_id TEXT PRIMARY KEY, order_id TEXT NOT NULL, event_channel TEXT NOT NULL, channel_sequence INTEGER NOT NULL, envelope BLOB NOT NULL, state TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(order_id, event_channel, channel_sequence));`); err != nil {
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
	return payload, nil
}
