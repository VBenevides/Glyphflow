package worker

import (
	"database/sql"
	"encoding/json"

	_ "modernc.org/sqlite"
)

type LocalStore struct{ db *sql.DB }

func OpenStore(path string) (*LocalStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &LocalStore{db: db}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, value BLOB NOT NULL);`); err != nil {
		_ = db.Close()
		return nil, err
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
