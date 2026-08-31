package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	SecretIntegrityUnknown          = "UNKNOWN"
	SecretIntegrityValid            = "VALID"
	SecretIntegrityFailed           = "INTEGRITY_FAILED"
	SecretIntegrityKeyUnavailable   = "KEY_UNAVAILABLE"
	SecretIntegrityDecryptionFailed = "DECRYPTION_FAILED"
)

var (
	ErrEncryptedSecretNotFound = errors.New("encrypted secret not found")
	ErrEncryptedSecretInUse    = errors.New("encrypted secret is in use")
)

type EncryptedSecretRecord struct {
	ID              string
	Name            string
	EncryptedValue  []byte
	IntegrityStatus string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastValidatedAt *time.Time
}

type EncryptedSecretStatusRecord struct {
	ID              string
	Name            string
	IntegrityStatus string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastValidatedAt *time.Time
	Tasks           []SecretTaskUsageRecord
	CanDelete       bool
}

type SecretTaskUsageRecord struct {
	ID   string
	Name string
}

type EncryptedSecretRepository interface {
	Upsert(context.Context, EncryptedSecretRecord) error
	Find(context.Context, string) (EncryptedSecretRecord, bool, error)
	SetIntegrityStatus(context.Context, string, string, time.Time) error
	ListStatuses(context.Context) ([]EncryptedSecretStatusRecord, error)
	Delete(context.Context, string) error
}

type EncryptedSecretStore struct{ pool database }

func NewEncryptedSecretRepository(pool any) *EncryptedSecretStore {
	db, _ := databaseFrom(pool)
	return &EncryptedSecretStore{pool: db}
}

func (s *EncryptedSecretStore) Upsert(ctx context.Context, secret EncryptedSecretRecord) error {
	secret.Name = strings.TrimSpace(secret.Name)
	if secret.Name == "" {
		secret.Name = secret.ID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO encrypted_secrets (id, name, encrypted_value, integrity_status)
		VALUES ($1, $2, $3, 'UNKNOWN')
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			encrypted_value = EXCLUDED.encrypted_value,
			integrity_status = 'UNKNOWN',
			updated_at = now(),
			last_validated_at = NULL`, secret.ID, secret.Name, secret.EncryptedValue)
	return err
}

func (s *EncryptedSecretStore) Find(ctx context.Context, id string) (EncryptedSecretRecord, bool, error) {
	var secret EncryptedSecretRecord
	err := s.pool.QueryRow(ctx, `SELECT id, name, encrypted_value, integrity_status, created_at, updated_at, last_validated_at FROM encrypted_secrets WHERE id = $1`, id).
		Scan(&secret.ID, &secret.Name, &secret.EncryptedValue, &secret.IntegrityStatus, &secret.CreatedAt, &secret.UpdatedAt, &secret.LastValidatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EncryptedSecretRecord{}, false, nil
	}
	return secret, err == nil, err
}

func (s *EncryptedSecretStore) SetIntegrityStatus(ctx context.Context, id, status string, validatedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE encrypted_secrets SET integrity_status = $2, updated_at = now(), last_validated_at = $3 WHERE id = $1`, id, status, validatedAt)
	return err
}

func (s *EncryptedSecretStore) Delete(ctx context.Context, id string) error {
	var deleted string
	err := s.pool.QueryRow(ctx, `
		DELETE FROM encrypted_secrets AS secret
		WHERE secret.id = $1
		  AND NOT EXISTS (
			SELECT 1
			FROM tasks AS task
			JOIN task_versions AS version ON version.id = task.current_version_id AND version.task_id = task.id
			CROSS JOIN LATERAL jsonb_each_text(version.secret_references) AS reference(name, value)
			WHERE NOT task.is_deleted AND reference.value = secret.id
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM sso_providers AS provider
			WHERE 'oidc-provider:' || provider.id = secret.id
		  )
		RETURNING secret.id`, id).Scan(&deleted)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM encrypted_secrets WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrEncryptedSecretNotFound
	}
	return ErrEncryptedSecretInUse
}

func (s *EncryptedSecretStore) ListStatuses(ctx context.Context) ([]EncryptedSecretStatusRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT secret.id, secret.name, secret.integrity_status, secret.created_at, secret.updated_at, secret.last_validated_at,
			COALESCE(task_usage.tasks, '[]'::jsonb),
			COALESCE(task_usage.task_count, 0) = 0 AND NOT EXISTS (
				SELECT 1 FROM sso_providers AS provider
				WHERE 'oidc-provider:' || provider.id = secret.id
			) AS can_delete
		FROM encrypted_secrets AS secret
		LEFT JOIN (
			SELECT usage.secret_id,
				jsonb_agg(jsonb_build_object('id', usage.id, 'name', usage.name) ORDER BY lower(usage.name), usage.id) AS tasks,
				COUNT(*) AS task_count
			FROM (
				SELECT DISTINCT secret.id AS secret_id, task.id, task.name
				FROM encrypted_secrets AS secret
				JOIN tasks AS task ON NOT task.is_deleted
				JOIN task_versions AS version ON version.id = task.current_version_id AND version.task_id = task.id
				CROSS JOIN jsonb_each_text(version.secret_references) AS reference
				WHERE reference.value = secret.id
			) AS usage
			GROUP BY usage.secret_id
		) AS task_usage ON task_usage.secret_id = secret.id
		ORDER BY lower(secret.name), secret.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := []EncryptedSecretStatusRecord{}
	for rows.Next() {
		var status EncryptedSecretStatusRecord
		var tasks []byte
		if err := rows.Scan(&status.ID, &status.Name, &status.IntegrityStatus, &status.CreatedAt, &status.UpdatedAt, &status.LastValidatedAt, &tasks, &status.CanDelete); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tasks, &status.Tasks); err != nil {
			return nil, err
		}
		if status.Tasks == nil {
			status.Tasks = []SecretTaskUsageRecord{}
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}
