ALTER TABLE runner_pools ADD COLUMN IF NOT EXISTS is_deleted boolean NOT NULL DEFAULT false;
DROP INDEX IF EXISTS runner_pools_name_ci_idx;
CREATE UNIQUE INDEX runner_pools_name_ci_idx ON runner_pools (lower(name)) WHERE NOT is_deleted;
CREATE INDEX IF NOT EXISTS runner_pools_active_idx ON runner_pools (lower(name), id) WHERE NOT is_deleted;
