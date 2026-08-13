CREATE TABLE event_inbox (
    event_id text PRIMARY KEY,
    task_run_id text NOT NULL REFERENCES task_runs(id),
    attempt integer NOT NULL CHECK (attempt > 0),
    received_at timestamptz NOT NULL DEFAULT now()
);
