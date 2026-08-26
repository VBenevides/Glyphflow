CREATE TABLE dead_letters (
    id text PRIMARY KEY,
    runner_id text NOT NULL DEFAULT '',
    stream text NOT NULL,
    consumer text NOT NULL,
    subject text NOT NULL,
    message_id text NOT NULL,
    payload_ciphertext bytea NOT NULL CHECK (octet_length(payload_ciphertext) > 0),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    error_text text NOT NULL DEFAULT '' CHECK (octet_length(error_text) <= 4096),
    attempts integer NOT NULL CHECK (attempts > 0),
    first_failed_at timestamptz NOT NULL,
    last_failed_at timestamptz NOT NULL,
    correlation_id text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN', 'RETRY_QUEUED', 'RECONCILED', 'DISCARDED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (stream, consumer, message_id)
);
CREATE INDEX dead_letters_state_idx ON dead_letters(state, last_failed_at DESC);
