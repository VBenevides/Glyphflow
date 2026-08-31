package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type OIDCProviderRecord struct {
	ID, Name, Issuer, ClientID     string
	CallbackURLs                   []string
	AuthEndpointOverride, Audience string
	Enabled, AutoProvision         bool
}

type OIDCProviderRepository interface {
	Upsert(context.Context, OIDCProviderRecord) error
	List(context.Context) ([]OIDCProviderRecord, error)
	Find(context.Context, string) (OIDCProviderRecord, bool, error)
	EnabledCount(context.Context) (int, error)
	ListGroupRoleMappings(context.Context, string) ([]SSOGroupRoleMappingRecord, error)
	SetGroupRoleMapping(context.Context, SSOGroupRoleMappingRecord) error
	ReplaceGroupRoleMappings(context.Context, string, []SSOGroupRoleMappingRecord) error
}

type SSOIdentityRecord struct {
	ID, UserID, ProviderID, Subject string
}

type SSOGroupRoleMappingRecord struct {
	ProviderID, GroupName, RoleID string
}

type SSORepository interface {
	FindIdentity(context.Context, string, string) (SSOIdentityRecord, bool, error)
	ListIdentities(context.Context, string) ([]SSOIdentityRecord, error)
	CreateIdentity(context.Context, SSOIdentityRecord) error
	DeleteIdentity(context.Context, string, string, string) error
	ListGroupRoleMappings(context.Context, string) ([]SSOGroupRoleMappingRecord, error)
	SetGroupRoleMapping(context.Context, SSOGroupRoleMappingRecord) error
	DeleteGroupRoleMapping(context.Context, string, string, string) error
}

type OIDCProvisioner interface {
	ProvisionOIDC(context.Context, UserRecord, string, string, SSOIdentityRecord) error
}

type OIDCProviderStore struct{ pool database }

func NewOIDCProviderRepository(pool any) *OIDCProviderStore {
	db, _ := databaseFrom(pool)
	return &OIDCProviderStore{pool: db}
}

func (s *OIDCProviderStore) ProvisionOIDC(ctx context.Context, user UserRecord, defaultRoleID, adminRoleID string, identity SSOIdentityRecord) error {
	user.DisplayName = NormalizeDisplayName(user.Email, user.DisplayName)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, err := userStatus(user)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, username, email, display_name, status) VALUES ($1, $2, $3, $4, $5)`, user.ID, user.Username, user.Email, user.DisplayName, status); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, 'default', $2)`, user.ID, defaultRoleID); err != nil {
		return err
	}
	if adminRoleID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, 'system-admin', $1)`, user.ID, adminRoleID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_sso_identities (id, user_id, provider_id, provider_subject) VALUES ($1, $2, $3, $4)`, identity.ID, identity.UserID, identity.ProviderID, identity.Subject); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *OIDCProviderStore) Upsert(ctx context.Context, provider OIDCProviderRecord) error {
	callbacks, err := json.Marshal(provider.CallbackURLs)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sso_providers
			(id, name, issuer, client_id, callback_urls, auth_endpoint_override, audience, enabled, auto_provision)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			issuer = EXCLUDED.issuer,
			client_id = EXCLUDED.client_id,
			callback_urls = EXCLUDED.callback_urls,
			auth_endpoint_override = EXCLUDED.auth_endpoint_override,
			audience = EXCLUDED.audience,
			enabled = EXCLUDED.enabled,
			auto_provision = EXCLUDED.auto_provision,
			updated_at = now()`,
		provider.ID, provider.Name, provider.Issuer, provider.ClientID, callbacks, provider.AuthEndpointOverride,
		provider.Audience, provider.Enabled, provider.AutoProvision)
	return err
}

func (s *OIDCProviderStore) List(ctx context.Context) ([]OIDCProviderRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, issuer, client_id, callback_urls, auth_endpoint_override, audience, enabled, auto_provision FROM sso_providers ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []OIDCProviderRecord{}
	for rows.Next() {
		var provider OIDCProviderRecord
		var callbacks []byte
		if err := rows.Scan(&provider.ID, &provider.Name, &provider.Issuer, &provider.ClientID, &callbacks, &provider.AuthEndpointOverride, &provider.Audience, &provider.Enabled, &provider.AutoProvision); err != nil {
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
	err := s.pool.QueryRow(ctx, `SELECT id, name, issuer, client_id, callback_urls, auth_endpoint_override, audience, enabled, auto_provision FROM sso_providers WHERE id = $1`, id).
		Scan(&provider.ID, &provider.Name, &provider.Issuer, &provider.ClientID, &callbacks, &provider.AuthEndpointOverride, &provider.Audience, &provider.Enabled, &provider.AutoProvision)
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

func (s *OIDCProviderStore) FindIdentity(ctx context.Context, providerID, subject string) (SSOIdentityRecord, bool, error) {
	var identity SSOIdentityRecord
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, provider_id, provider_subject FROM user_sso_identities WHERE provider_id = $1 AND provider_subject = $2`, providerID, subject).Scan(&identity.ID, &identity.UserID, &identity.ProviderID, &identity.Subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return SSOIdentityRecord{}, false, nil
	}
	return identity, err == nil, err
}

func (s *OIDCProviderStore) ListIdentities(ctx context.Context, userID string) ([]SSOIdentityRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, provider_id, provider_subject FROM user_sso_identities WHERE user_id = $1 ORDER BY provider_id, provider_subject`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := []SSOIdentityRecord{}
	for rows.Next() {
		var identity SSOIdentityRecord
		if err := rows.Scan(&identity.ID, &identity.UserID, &identity.ProviderID, &identity.Subject); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

func (s *OIDCProviderStore) CreateIdentity(ctx context.Context, identity SSOIdentityRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_sso_identities (id, user_id, provider_id, provider_subject) VALUES ($1, $2, $3, $4)`, identity.ID, identity.UserID, identity.ProviderID, identity.Subject)
	return err
}

func (s *OIDCProviderStore) DeleteIdentity(ctx context.Context, userID, providerID, subject string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM user_sso_identities WHERE user_id = $1 AND provider_id = $2 AND provider_subject = $3`, userID, providerID, subject)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("identity not found")
	}
	return nil
}

func (s *OIDCProviderStore) ListGroupRoleMappings(ctx context.Context, providerID string) ([]SSOGroupRoleMappingRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT provider_id, group_name, role_id FROM sso_group_role_mappings WHERE provider_id = $1 ORDER BY group_name, role_id`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	mappings := []SSOGroupRoleMappingRecord{}
	for rows.Next() {
		var mapping SSOGroupRoleMappingRecord
		if err := rows.Scan(&mapping.ProviderID, &mapping.GroupName, &mapping.RoleID); err != nil {
			return nil, err
		}
		mappings = append(mappings, mapping)
	}
	return mappings, rows.Err()
}

func (s *OIDCProviderStore) SetGroupRoleMapping(ctx context.Context, mapping SSOGroupRoleMappingRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sso_group_role_mappings (provider_id, group_name, role_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, mapping.ProviderID, mapping.GroupName, mapping.RoleID)
	return err
}

func (s *OIDCProviderStore) ReplaceGroupRoleMappings(ctx context.Context, providerID string, mappings []SSOGroupRoleMappingRecord) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM sso_group_role_mappings WHERE provider_id = $1`, providerID); err != nil {
		return err
	}
	for _, mapping := range mappings {
		if mapping.GroupName == "" || mapping.RoleID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sso_group_role_mappings (provider_id, group_name, role_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, providerID, mapping.GroupName, mapping.RoleID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *OIDCProviderStore) DeleteGroupRoleMapping(ctx context.Context, providerID, groupName, roleID string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM sso_group_role_mappings WHERE provider_id = $1 AND group_name = $2 AND role_id = $3`, providerID, groupName, roleID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("mapping not found")
	}
	return nil
}
