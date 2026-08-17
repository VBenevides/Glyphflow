ALTER TABLE sso_authorization_states
    ADD COLUMN IF NOT EXISTS link_user_id text REFERENCES users(id) ON DELETE CASCADE;
