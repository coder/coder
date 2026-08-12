CREATE TYPE aibridge_interception_error_type AS ENUM (
    'bad_request',
    'unauthorized',
    'rate_limited',
    'overloaded',
    'server_error',
    'timeout',
    'unknown'
);

-- Records the terminal upstream error observed when an interception failed.
-- Both columns are NULL for interceptions that completed successfully.
-- error_message is capped at 1024 characters as a hard schema-level bound.
ALTER TABLE aibridge_interceptions
    ADD COLUMN error_type aibridge_interception_error_type,
    ADD COLUMN error_message varchar(1024);

COMMENT ON COLUMN aibridge_interceptions.error_type IS 'Categorised terminal upstream error for a failed interception; NULL when the interception succeeded.';
COMMENT ON COLUMN aibridge_interceptions.error_message IS 'Raw terminal upstream error message for a failed interception; NULL when the interception succeeded.';
