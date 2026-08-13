CREATE TABLE task_runs (
    id text PRIMARY KEY,
    task_definition_id text NOT NULL REFERENCES task_definitions(id),
    occurrence_at timestamptz NOT NULL,
    runner_id text,
    state text NOT NULL DEFAULT 'queued',
    attempt integer NOT NULL DEFAULT 1 CHECK (attempt > 0),
    lease_token text,
    queued_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    result jsonb,
    state_version bigint NOT NULL DEFAULT 0 CHECK (state_version >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
