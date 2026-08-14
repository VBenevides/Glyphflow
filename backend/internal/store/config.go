package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConfigStore struct {
	pool *pgxpool.Pool
}

func NewConfigStore(pool *pgxpool.Pool) *ConfigStore {
	return &ConfigStore{pool: pool}
}

func (s *ConfigStore) Get(ctx context.Context, name string, target any) (bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM config WHERE name = $1`, name).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, err
	}
	return true, nil
}

func (s *ConfigStore) Set(ctx context.Context, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO config (name, value) VALUES ($1, $2::jsonb) ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`, name, raw)
	return err
}

func (s *ConfigStore) SetIfAbsent(ctx context.Context, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO config (name, value) VALUES ($1, $2::jsonb) ON CONFLICT (name) DO NOTHING`, name, raw)
	return err
}
