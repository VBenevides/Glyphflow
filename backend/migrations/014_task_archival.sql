ALTER TABLE tasks ADD COLUMN IF NOT EXISTS is_deleted boolean NOT NULL DEFAULT false;
UPDATE tasks SET is_deleted = deleted WHERE deleted AND NOT is_deleted;
CREATE INDEX IF NOT EXISTS tasks_active_idx ON tasks (lower(name), id) WHERE NOT is_deleted;
DROP TRIGGER IF EXISTS task_versions_immutable ON task_versions;
CREATE TRIGGER task_versions_immutable
    BEFORE UPDATE OR DELETE ON task_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_version_mutation();
