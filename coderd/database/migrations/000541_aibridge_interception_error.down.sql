ALTER TABLE aibridge_interceptions
    DROP COLUMN error_type,
    DROP COLUMN error_message;

DROP TYPE aibridge_interception_error_type;
