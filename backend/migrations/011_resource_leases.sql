CREATE TABLE resource_leases (
    id text PRIMARY KEY,
    resource_key text NOT NULL,
    task_run_id text NOT NULL REFERENCES task_runs(id),
    lease_token text NOT NULL,
    expires_at timestamptz NOT NULL,
    released_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX resource_leases_active_resource_idx
    ON resource_leases (resource_key)
    WHERE released_at IS NULL;
