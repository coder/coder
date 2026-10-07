CREATE TYPE connection_log_method AS ENUM (
	'ssh', 'reconnecting_pty', 'workspace_app', 'port_forwarding', 'tunnel'
);

-- Convert both columns in one rewrite under an ACCESS EXCLUSIVE lock.
ALTER TABLE connection_logs
	ALTER COLUMN slug_or_port TYPE text USING (
		CASE
			WHEN type::text IN ('vscode', 'jetbrains') THEN type::text
			WHEN type::text IN ('ssh', 'reconnecting_pty', 'tunnel') THEN NULL
			ELSE NULLIF(slug_or_port, '')
		END
	),
	ALTER COLUMN type TYPE connection_log_method USING (
		CASE
			WHEN type::text IN ('vscode', 'jetbrains') THEN 'ssh'
			ELSE type::text
		END
	)::connection_log_method;

ALTER TABLE connection_logs RENAME COLUMN type TO connection_method;
ALTER TABLE connection_logs RENAME COLUMN slug_or_port TO app_name_or_port;
DROP TYPE connection_type;

COMMENT ON COLUMN connection_logs.connection_method IS 'How the connection was established.';
COMMENT ON COLUMN connection_logs.app_name_or_port IS 'Client identity for SSH and reconnecting PTY; destination slug or port for workspace apps and port forwarding. Null when absent.';
