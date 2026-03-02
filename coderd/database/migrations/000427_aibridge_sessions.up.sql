ALTER TABLE aibridge_interceptions
ADD COLUMN client_session_id VARCHAR(100) NULL; -- Limit session ID to a short string to prevent accidentally indexing large strings.

COMMENT ON COLUMN aibridge_interceptions.client_session_id IS 'The session ID supplied by the client (optional and not universally supported).';

CREATE INDEX idx_aibridge_interceptions_client_session_id ON aibridge_interceptions (client_session_id);
