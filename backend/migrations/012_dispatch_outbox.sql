CREATE TABLE dispatch_outbox (
    id text PRIMARY KEY,
    task_run_id text NOT NULL REFERENCES task_runs(id),
    order_bytes bytea NOT NULL CHECK (octet_length(order_bytes) > 0),
    subject text NOT NULL,
    state text NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);
