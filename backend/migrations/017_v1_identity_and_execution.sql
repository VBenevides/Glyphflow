CREATE TABLE IF NOT EXISTS users_v1 (
    id text PRIMARY KEY,
    username text NOT NULL,
    email text,
    display_name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','pending','disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_v1_username_ci_idx ON users_v1 (lower(username));
CREATE UNIQUE INDEX users_v1_email_ci_idx ON users_v1 (lower(email)) WHERE email IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_passwords_v1 (
    user_id text PRIMARY KEY REFERENCES users_v1(id) ON DELETE CASCADE,
    password_hash text NOT NULL,
    password_changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS auth_sessions_v1 (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users_v1(id) ON DELETE CASCADE,
    refresh_token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_sessions_v1_user_idx ON auth_sessions_v1(user_id, expires_at);

CREATE TABLE IF NOT EXISTS roles_v1 (
    id text PRIMARY KEY,
    key text NOT NULL UNIQUE,
    description text NOT NULL,
    is_system boolean NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS permissions_v1 (
    id text PRIMARY KEY,
    key text NOT NULL UNIQUE,
    description text NOT NULL
);
CREATE TABLE IF NOT EXISTS role_permissions_v1 (
    role_id text NOT NULL REFERENCES roles_v1(id) ON DELETE CASCADE,
    permission_id text NOT NULL REFERENCES permissions_v1(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);
CREATE TABLE IF NOT EXISTS role_assignments_v1 (
    user_id text NOT NULL REFERENCES users_v1(id) ON DELETE CASCADE,
    role_id text NOT NULL REFERENCES roles_v1(id) ON DELETE RESTRICT,
    source_type text NOT NULL,
    source_key text NOT NULL,
    PRIMARY KEY (user_id, role_id, source_type, source_key)
);

CREATE TABLE IF NOT EXISTS sso_providers_v1 (
    id text PRIMARY KEY,
    key text NOT NULL UNIQUE,
    issuer text NOT NULL,
    client_id text NOT NULL,
    callback_urls jsonb NOT NULL DEFAULT '[]'::jsonb,
    enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS user_sso_identities_v1 (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users_v1(id) ON DELETE CASCADE,
    provider_id text NOT NULL REFERENCES sso_providers_v1(id) ON DELETE CASCADE,
    provider_subject text NOT NULL,
    UNIQUE (provider_id, provider_subject)
);
CREATE TABLE IF NOT EXISTS sso_authorization_states_v1 (
    id text PRIMARY KEY,
    provider_id text NOT NULL REFERENCES sso_providers_v1(id) ON DELETE CASCADE,
    state_hash text NOT NULL UNIQUE,
    nonce_hash text NOT NULL,
    purpose text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);

CREATE TABLE IF NOT EXISTS audit_events_v1 (
    id text PRIMARY KEY,
    actor_type text NOT NULL,
    actor_id text,
    session_id text,
    endpoint text NOT NULL,
    target_type text,
    target_id text,
    result text NOT NULL,
    before_value jsonb,
    after_value jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_v1_created_idx ON audit_events_v1(created_at DESC);
