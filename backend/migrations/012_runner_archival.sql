ALTER TABLE runners ADD COLUMN IF NOT EXISTS is_archived boolean NOT NULL DEFAULT false;
ALTER TABLE runners ADD COLUMN IF NOT EXISTS is_deleted boolean NOT NULL DEFAULT false;
ALTER TABLE runners ALTER COLUMN pool_id DROP NOT NULL;
ALTER TABLE runners DROP CONSTRAINT IF EXISTS runners_pool_id_fkey;
ALTER TABLE runners ADD CONSTRAINT runners_pool_id_fkey FOREIGN KEY (pool_id) REFERENCES runner_pools(id) ON DELETE SET NULL;
DROP INDEX IF EXISTS runners_name_ci_idx;
CREATE UNIQUE INDEX runners_name_ci_idx ON runners (lower(name)) WHERE NOT is_archived AND NOT is_deleted;
