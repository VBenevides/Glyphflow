CREATE TABLE task_definitions (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    schedule text NOT NULL,
    timezone text NOT NULL,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    command jsonb NOT NULL CHECK (jsonb_typeof(command) = 'array'),
    selectors jsonb NOT NULL DEFAULT '{}'::jsonb,
    resources jsonb NOT NULL DEFAULT '{}'::jsonb,
    retry_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled boolean NOT NULL DEFAULT true,
    next_due_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
