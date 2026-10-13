CREATE TYPE connection_type AS ENUM (
	'ssh', 'vscode', 'jetbrains', 'reconnecting_pty',
	'workspace_app', 'port_forwarding', 'tunnel'
);

-- App names collapse to the historical SSH families on rollback.
ALTER TABLE connection_logs
	ALTER COLUMN app_name_or_port TYPE text USING (
		CASE
			WHEN connection_method IN ('ssh', 'reconnecting_pty', 'tunnel') THEN NULL
			ELSE app_name_or_port
		END
	),
	ALTER COLUMN connection_method TYPE connection_type USING (
		CASE
			WHEN connection_method != 'ssh' THEN connection_method::text
			-- Snapshot of the VS Code and JetBrains families in
			-- codersdk/appname.go. Apps registered later roll back to ssh.
			WHEN app_name_or_port IN (
				'vscode', 'vscode_insiders', 'vscode_web', 'code_server', 'cursor',
				'windsurf', 'positron', 'vscodium', 'codium', 'antigravity', 'trae',
				'kiro', 'devin', 'code_oss', 'vscodium_insiders', 'devin_next', 'trae_cn'
			) THEN 'vscode'
			WHEN app_name_or_port IN (
				'jetbrains', 'intellij', 'pycharm', 'goland', 'webstorm', 'phpstorm',
				'rubymine', 'clion', 'rider', 'rustrover', 'datagrip', 'dataspell',
				'mps', 'android_studio'
			) THEN 'jetbrains'
			ELSE 'ssh'
		END
	)::connection_type;

COMMENT ON COLUMN connection_logs.connection_method IS NULL;
COMMENT ON COLUMN connection_logs.app_name_or_port IS 'Null for SSH events. For web connections, this is the slug of the app or the port number being forwarded.';
ALTER TABLE connection_logs RENAME COLUMN app_name_or_port TO slug_or_port;
ALTER TABLE connection_logs RENAME COLUMN connection_method TO type;
DROP TYPE connection_log_method;
