package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	SecretIntegrityUnknown          = "UNKNOWN"
	SecretIntegrityValid            = "VALID"
	SecretIntegrityFailed           = "INTEGRITY_FAILED"
	SecretIntegrityKeyUnavailable   = "KEY_UNAVAILABLE"
	SecretIntegrityDecryptionFailed = "DECRYPTION_FAILED"
)

type EncryptedSecretRecord struct {
	ID              string
	EncryptedValue  []byte
	IntegrityStatus string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastValidatedAt *time.Time
}

type EncryptedSecretStatusRecord struct {
	ID              string
	IntegrityStatus string
	LastValidatedAt *time.Time
}

type EncryptedSecretRepository interface {
	Upsert(context.Context, EncryptedSecretRecord) error
	Find(context.Context, string) (EncryptedSecretRecord, bool, error)
	SetIntegrityStatus(context.Context, string, string, time.Time) error
	ListStatuses(context.Context) ([]EncryptedSecretStatusRecord, error)
}

type EncryptedSecretStore struct{ pool *pgxpool.Pool }

func NewEncryptedSecretRepository(pool *pgxpool.Pool) *EncryptedSecretStore {
	return &EncryptedSecretStore{pool: pool}
}

func (s *EncryptedSecretStore) Upsert(ctx context.Context, secret EncryptedSecretRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO encrypted_secrets (id, encrypted_value, integrity_status)
		VALUES ($1, $2, 'UNKNOWN')
		ON CONFLICT (id) DO UPDATE SET
			encrypted_value = EXCLUDED.encrypted_value,
			integrity_status = 'UNKNOWN',
			updated_at = now(),
			last_validated_at = NULL`, secret.ID, secret.EncryptedValue)
	return err
}

func (s *EncryptedSecretStore) Find(ctx context.Context, id string) (EncryptedSecretRecord, bool, error) {
	var secret EncryptedSecretRecord
	err := s.pool.QueryRow(ctx, `SELECT id, encrypted_value, integrity_status, created_at, updated_at, last_validated_at FROM encrypted_secrets WHERE id = $1`, id).
		Scan(&secret.ID, &secret.EncryptedValue, &secret.IntegrityStatus, &secret.CreatedAt, &secret.UpdatedAt, &secret.LastValidatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EncryptedSecretRecord{}, false, nil
	}
	return secret, err == nil, err
}

func (s *EncryptedSecretStore) SetIntegrityStatus(ctx context.Context, id, status string, validatedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE encrypted_secrets SET integrity_status = $2, updated_at = now(), last_validated_at = $3 WHERE id = $1`, id, status, validatedAt)
	return err
}

func (s *EncryptedSecretStore) ListStatuses(ctx context.Context) ([]EncryptedSecretStatusRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, integrity_status, last_validated_at FROM encrypted_secrets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := []EncryptedSecretStatusRecord{}
	for rows.Next() {
		var status EncryptedSecretStatusRecord
		if err := rows.Scan(&status.ID, &status.IntegrityStatus, &status.LastValidatedAt); err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}
