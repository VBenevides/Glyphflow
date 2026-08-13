CREATE TABLE runner_keys (
    key_id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners(id),
    public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
    not_before timestamptz NOT NULL,
    not_after timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (not_after IS NULL OR not_after > not_before)
);
