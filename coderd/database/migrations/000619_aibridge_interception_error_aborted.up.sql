ALTER TYPE aibridge_interception_error_type ADD VALUE IF NOT EXISTS 'aborted';

COMMENT ON COLUMN aibridge_interceptions.error_type IS 'Categorised terminal error for a failed interception, including aborted when the client went away before it completed; NULL when the interception succeeded.';
