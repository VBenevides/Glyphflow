CREATE TABLE runner_enrollments (
    id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners(id),
    token_hash bytea NOT NULL CHECK (octet_length(token_hash) > 0),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    requester text NOT NULL,
    target text NOT NULL,
    artifact jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
