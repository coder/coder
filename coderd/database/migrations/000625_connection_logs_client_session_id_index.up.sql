CREATE INDEX idx_connection_logs_client_session_id_connect_time
	ON connection_logs (client_session_id, connect_time DESC)
	WHERE client_session_id IS NOT NULL;
