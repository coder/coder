DROP INDEX IF EXISTS idx_aibridge_interceptions_client_session_id;

ALTER TABLE aibridge_interceptions
DROP COLUMN client_session_id;
