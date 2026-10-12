ALTER TABLE connection_logs
	  ADD COLUMN IF NOT EXISTS client_session_id text DEFAULT NULL
	  CHECK (client_session_id IS NULL OR client_session_id ~ '^[0-9a-f]{32}$');

COMMENT ON COLUMN connection_logs.client_session_id IS 'Tracks all connections over the lifetime of a single client (IDE or ssh) session. As it originates from the client, it is not guaranteed to be unique.';
