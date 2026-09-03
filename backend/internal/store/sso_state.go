package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type OIDCAuthorizationStateRecord struct {
	ID, ProviderID, StateHash, NonceHash string
	EncryptedPKCEVerifier                []byte
	Purpose, Callback, UserID            string
	ExpiresAt                            time.Time
	ConsumedAt                           *time.Time
}

type OIDCAuthorizationStateRepository interface {
	Create(context.Context, OIDCAuthorizationStateRecord) error
	Consume(context.Context, string, string, string, string, string, time.Time) (OIDCAuthorizationStateRecord, error)
	DeleteExpired(context.Context, time.Time) error
}

type OIDCAuthorizationStateAnyConsumer interface {
	ConsumeAny(context.Context, string, string, string, string, time.Time) (OIDCAuthorizationStateRecord, error)
}

type OIDCAuthorizationStateStore struct{ pool database }

func NewOIDCAuthorizationStateRepository(pool any) *OIDCAuthorizationStateStore {
	db, _ := databaseFrom(pool)
	return &OIDCAuthorizationStateStore{pool: db}
}

func (s *OIDCAuthorizationStateStore) Create(ctx context.Context, state OIDCAuthorizationStateRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sso_authorization_states (id, provider_id, state_hash, nonce_hash, encrypted_pkce_verifier, purpose, callback_url, link_user_id, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)`, state.ID, state.ProviderID, state.StateHash, state.NonceHash, state.EncryptedPKCEVerifier, state.Purpose, state.Callback, state.UserID, state.ExpiresAt)
	return err
}

func (s *OIDCAuthorizationStateStore) Consume(ctx context.Context, stateHash, nonceHash, providerID, purpose, callback string, now time.Time) (OIDCAuthorizationStateRecord, error) {
	return s.consume(ctx, stateHash, nonceHash, providerID, purpose, callback, now)
}

func (s *OIDCAuthorizationStateStore) ConsumeAny(ctx context.Context, stateHash, nonceHash, providerID, callback string, now time.Time) (OIDCAuthorizationStateRecord, error) {
	return s.consume(ctx, stateHash, nonceHash, providerID, "", callback, now)
}

func (s *OIDCAuthorizationStateStore) consume(ctx context.Context, stateHash, nonceHash, providerID, purpose, callback string, now time.Time) (OIDCAuthorizationStateRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OIDCAuthorizationStateRecord{}, err
	}
	defer tx.Rollback(ctx)
	var state OIDCAuthorizationStateRecord
	err = tx.QueryRow(ctx, `SELECT id, provider_id, state_hash, nonce_hash, encrypted_pkce_verifier, purpose, callback_url, COALESCE(link_user_id, ''), expires_at, consumed_at FROM sso_authorization_states WHERE state_hash = $1 FOR UPDATE`, stateHash).
		Scan(&state.ID, &state.ProviderID, &state.StateHash, &state.NonceHash, &state.EncryptedPKCEVerifier, &state.Purpose, &state.Callback, &state.UserID, &state.ExpiresAt, &state.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OIDCAuthorizationStateRecord{}, errors.New("authorization state is invalid")
	}
	if err != nil {
		return OIDCAuthorizationStateRecord{}, err
	}
	if state.ConsumedAt != nil || state.NonceHash != nonceHash || (providerID != "" && state.ProviderID != providerID) || (purpose != "" && state.Purpose != purpose) || (callback != "" && state.Callback != callback) || !now.Before(state.ExpiresAt) {
		return OIDCAuthorizationStateRecord{}, errors.New("authorization state is invalid")
	}
	if _, err := tx.Exec(ctx, `UPDATE sso_authorization_states SET consumed_at = now() WHERE id = $1`, state.ID); err != nil {
		return OIDCAuthorizationStateRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OIDCAuthorizationStateRecord{}, err
	}
	nowConsumed := now
	state.ConsumedAt = &nowConsumed
	return state, nil
}

func (s *OIDCAuthorizationStateStore) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sso_authorization_states WHERE expires_at <= $1 OR consumed_at IS NOT NULL`, now)
	return err
}
