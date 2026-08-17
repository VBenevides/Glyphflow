package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExitCodeRecord struct {
	Code     int    `json:"code"`
	Meaning  string `json:"meaning"`
	IsSystem bool   `json:"isSystem"`
}

type ExitCodeRepository interface {
	List(context.Context) ([]ExitCodeRecord, error)
	Create(context.Context, int, string) (ExitCodeRecord, error)
	Update(context.Context, int, int, string) (ExitCodeRecord, error)
	Delete(context.Context, int) error
}

type ExitCodeStore struct{ pool *pgxpool.Pool }

var (
	ErrExitCodeNotFound = errors.New("exit code not found")
	ErrExitCodeSystem   = errors.New("system exit code cannot be changed")
	ErrExitCodeExists   = errors.New("exit code already exists")
	ErrExitCodeInUse    = errors.New("exit code is used by an execution attempt")
)

func NewExitCodeRepository(pool *pgxpool.Pool) *ExitCodeStore { return &ExitCodeStore{pool: pool} }

func (s *ExitCodeStore) List(ctx context.Context) ([]ExitCodeRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, meaning, is_system FROM exit_code ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ExitCodeRecord{}
	for rows.Next() {
		var item ExitCodeRecord
		if err := rows.Scan(&item.Code, &item.Meaning, &item.IsSystem); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func validateExitCodeMeaning(meaning string) (string, error) {
	meaning = strings.TrimSpace(meaning)
	if meaning == "" {
		return "", errors.New("exit code meaning is required")
	}
	return meaning, nil
}

func (s *ExitCodeStore) Create(ctx context.Context, code int, meaning string) (ExitCodeRecord, error) {
	meaning, err := validateExitCodeMeaning(meaning)
	if err != nil {
		return ExitCodeRecord{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO exit_code (code, meaning, is_system) VALUES ($1, $2, false)`, code, meaning)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ExitCodeRecord{}, ErrExitCodeExists
	}
	if err != nil {
		return ExitCodeRecord{}, err
	}
	return ExitCodeRecord{Code: code, Meaning: meaning}, nil
}

func (s *ExitCodeStore) Update(ctx context.Context, code, newCode int, meaning string) (ExitCodeRecord, error) {
	meaning, err := validateExitCodeMeaning(meaning)
	if err != nil {
		return ExitCodeRecord{}, err
	}
	var isSystem bool
	if err := s.pool.QueryRow(ctx, `SELECT is_system FROM exit_code WHERE code = $1`, code).Scan(&isSystem); errors.Is(err, pgx.ErrNoRows) {
		return ExitCodeRecord{}, ErrExitCodeNotFound
	} else if err != nil {
		return ExitCodeRecord{}, err
	}
	if isSystem {
		return ExitCodeRecord{}, ErrExitCodeSystem
	}
	_, err = s.pool.Exec(ctx, `UPDATE exit_code SET code = $2, meaning = $3 WHERE code = $1 AND is_system = false`, code, newCode, meaning)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23503" {
			return ExitCodeRecord{}, ErrExitCodeInUse
		}
		if pgErr.Code == "23505" {
			return ExitCodeRecord{}, ErrExitCodeExists
		}
	}
	if err != nil {
		return ExitCodeRecord{}, err
	}
	return ExitCodeRecord{Code: newCode, Meaning: meaning}, nil
}

func (s *ExitCodeStore) Delete(ctx context.Context, code int) error {
	var isSystem bool
	if err := s.pool.QueryRow(ctx, `SELECT is_system FROM exit_code WHERE code = $1`, code).Scan(&isSystem); errors.Is(err, pgx.ErrNoRows) {
		return ErrExitCodeNotFound
	} else if err != nil {
		return err
	}
	if isSystem {
		return ErrExitCodeSystem
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM exit_code WHERE code = $1 AND is_system = false`, code)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrExitCodeInUse
	}
	return err
}
