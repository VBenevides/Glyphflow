ALTER TABLE dead_letters
    ADD COLUMN retry_delivery_id text NOT NULL DEFAULT '',
    ADD COLUMN retry_attempts integer NOT NULL DEFAULT 0 CHECK (retry_attempts >= 0),
    ADD COLUMN retry_available_at timestamptz,
    ADD COLUMN retry_published_at timestamptz,
    ADD COLUMN retry_last_error text NOT NULL DEFAULT '' CHECK (octet_length(retry_last_error) <= 4096);
