ALTER TABLE runners DROP CONSTRAINT IF EXISTS runners_observed_state_check;
ALTER TABLE runners ADD CONSTRAINT runners_observed_state_check CHECK (observed_state IN ('PENDING', 'ONLINE', 'OFFLINE', 'REVOKED'));
ALTER TABLE runners ALTER COLUMN observed_state SET DEFAULT 'PENDING';
