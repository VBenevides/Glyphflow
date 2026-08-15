DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM schedule_versions WHERE schedule_type <> 'cron') THEN
        RAISE EXCEPTION 'convert or remove interval schedules before enabling cron-only scheduling';
    END IF;
END $$;
ALTER TABLE schedule_versions DROP COLUMN schedule_type;
