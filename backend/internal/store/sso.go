package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OIDCProviderRecord struct {
	ID, Name, Issuer, ClientID, SecretReference string
	CallbackURLs                                []string
	AuthEndpointOverride, Audience              string
	Enabled, AutoProvision                      bool
}

type OIDCProviderRepository interface {
	Upsert(context.Context, OIDCProviderRecord) error
	List(context.Context) ([]OIDCProviderRecord, error)
	Find(context.Context, string) (OIDCProviderRecord, bool, error)
	EnabledCount(context.Context) (int, error)
}

type OIDCProviderStore struct{ pool *pgxpool.Pool }

func NewOIDCProviderRepository(pool *pgxpool.Pool) *OIDCProviderStore {
	return &OIDCProviderStore{pool: pool}
}

func (s *OIDCProviderStore) Upsert(ctx context.Context, provider OIDCProviderRecord) error {
	callbacks, err := json.Marshal(provider.CallbackURLs)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sso_providers
			(id, name, issuer, client_id, secret_reference, callback_urls, auth_endpoint_override, audience, enabled, auto_provision)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			issuer = EXCLUDED.issuer,
			client_id = EXCLUDED.client_id,
			secret_reference = EXCLUDED.secret_reference,
			callback_urls = EXCLUDED.callback_urls,
			auth_endpoint_override = EXCLUDED.auth_endpoint_override,
			audience = EXCLUDED.audience,
			enabled = EXCLUDED.enabled,
			auto_provision = EXCLUDED.auto_provision,
			updated_at = now()`,
		provider.ID, provider.Name, provider.Issuer, provider.ClientID, provider.SecretReference, callbacks,
		provider.AuthEndpointOverride, provider.Audience, provider.Enabled, provider.AutoProvision)
	return err
}

func (s *OIDCProviderStore) List(ctx context.Context) ([]OIDCProviderRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, issuer, client_id, secret_reference, callback_urls, auth_endpoint_override, audience, enabled, auto_provision FROM sso_providers ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []OIDCProviderRecord{}
	for rows.Next() {
		var provider OIDCProviderRecord
		var callbacks []byte
		if err := rows.Scan(&provider.ID, &provider.Name, &provider.Issuer, &provider.ClientID, &provider.SecretReference, &callbacks, &provider.AuthEndpointOverride, &provider.Audience, &provider.Enabled, &provider.AutoProvision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(callbacks, &provider.CallbackURLs); err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, rows.Err()
}

func (s *OIDCProviderStore) Find(ctx context.Context, id string) (OIDCProviderRecord, bool, error) {
	var provider OIDCProviderRecord
	var callbacks []byte
	err := s.pool.QueryRow(ctx, `SELECT id, name, issuer, client_id, secret_reference, callback_urls, auth_endpoint_override, audience, enabled, auto_provision FROM sso_providers WHERE id = $1`, id).
		Scan(&provider.ID, &provider.Name, &provider.Issuer, &provider.ClientID, &provider.SecretReference, &callbacks, &provider.AuthEndpointOverride, &provider.Audience, &provider.Enabled, &provider.AutoProvision)
	if errors.Is(err, pgx.ErrNoRows) {
		return OIDCProviderRecord{}, false, nil
	}
	if err != nil {
		return OIDCProviderRecord{}, false, err
	}
	if err := json.Unmarshal(callbacks, &provider.CallbackURLs); err != nil {
		return OIDCProviderRecord{}, false, err
	}
	return provider, true, nil
}

func (s *OIDCProviderStore) EnabledCount(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM sso_providers WHERE enabled`).Scan(&count)
	return count, err
}
