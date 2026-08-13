CREATE TABLE runners (
    id text PRIMARY KEY,
    pool text NOT NULL,
    capacity integer NOT NULL DEFAULT 1 CHECK (capacity > 0),
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    state text NOT NULL DEFAULT 'enrolling',
    heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
