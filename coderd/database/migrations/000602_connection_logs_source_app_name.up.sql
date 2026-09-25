-- A single ALTER rewrites the table once.
ALTER TABLE connection_logs
	ALTER COLUMN slug_or_port TYPE text USING (
		CASE
			WHEN type IN ('ssh', 'vscode', 'jetbrains', 'reconnecting_pty') THEN type::text
			ELSE slug_or_port
		END
	),
	ALTER COLUMN type TYPE text USING (
		CASE
			WHEN type IN ('ssh', 'vscode', 'jetbrains', 'reconnecting_pty') THEN 'agent'
			ELSE type::text
		END
	);

ALTER TABLE connection_logs
	RENAME COLUMN type TO source;

ALTER TABLE connection_logs
	RENAME COLUMN slug_or_port TO app_name_or_port;

DROP TYPE connection_type;

COMMENT ON COLUMN connection_logs.source IS 'What logged the connection, such as agent or workspace_app.';

COMMENT ON COLUMN connection_logs.app_name_or_port IS 'Null for tunnels. For agent connections, this is the reported app name. For web connections, this is the slug of the app or the port number being forwarded.';
