DROP TRIGGER IF EXISTS task_versions_immutable ON task_versions;
CREATE TRIGGER task_versions_immutable
    BEFORE UPDATE ON task_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_version_mutation();
