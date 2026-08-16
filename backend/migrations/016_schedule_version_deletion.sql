DROP TRIGGER IF EXISTS schedule_versions_immutable ON schedule_versions;
CREATE TRIGGER schedule_versions_immutable
    BEFORE UPDATE ON schedule_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_version_mutation();
