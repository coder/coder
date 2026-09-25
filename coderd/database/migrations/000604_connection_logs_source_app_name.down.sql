CREATE TYPE connection_type AS ENUM (
	'ssh',
	'vscode',
	'jetbrains',
	'reconnecting_pty',
	'workspace_app',
	'port_forwarding',
	'tunnel'
);

COMMENT ON COLUMN connection_logs.source IS NULL;

COMMENT ON COLUMN connection_logs.app_name_or_port IS 'Null for SSH events. For web connections, this is the slug of the app or the port number being forwarded.';

-- Fold app names into families, using a snapshot of the app registry.
ALTER TABLE connection_logs
	ALTER COLUMN app_name_or_port TYPE text USING (
		CASE
			WHEN source = 'agent' THEN NULL
			ELSE app_name_or_port
		END
	),
	ALTER COLUMN source TYPE connection_type USING (
		CASE
			WHEN source IN ('workspace_app', 'port_forwarding', 'tunnel') THEN source
			WHEN app_name_or_port IN (
				'vscode', 'vscode_insiders', 'vscode_web', 'code_server', 'cursor',
				'windsurf', 'positron', 'vscodium', 'codium', 'antigravity', 'trae',
				'kiro', 'devin'
			) THEN 'vscode'
			WHEN app_name_or_port IN ('jetbrains', 'reconnecting_pty') THEN app_name_or_port
			ELSE 'ssh'
		END
	)::connection_type;

ALTER TABLE connection_logs
	RENAME COLUMN app_name_or_port TO slug_or_port;

ALTER TABLE connection_logs
	RENAME COLUMN source TO type;
