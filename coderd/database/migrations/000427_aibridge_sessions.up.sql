ALTER TABLE aibridge_interceptions
ADD COLUMN client_session_id TEXT NULL;

COMMENT ON COLUMN aibridge_interceptions.client_session_id IS 'The session ID supplied by the client (optional and not universally supported).';

-- Limit indexed session ID to a short string to prevent accidentally indexing large strings.
-- If we later implement a way to search by a session ID, and that session ID isn't fully indexed,
-- the lookup would still work but be unassisted by the index.
CREATE INDEX idx_aibridge_interceptions_client_session_id ON aibridge_interceptions (LEFT(client_session_id, 100))
WHERE client_session_id IS NOT NULL;
