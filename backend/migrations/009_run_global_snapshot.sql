ALTER TABLE runs
    ADD COLUMN IF NOT EXISTS resolved_global_variables jsonb NOT NULL DEFAULT '{}'::jsonb;
