-- Fold app names into families using a snapshot of the Go registry. Other
-- names stay ssh.
UPDATE connection_logs
SET
	kind = COALESCE((
		SELECT registry.family::connection_kind
		FROM (VALUES
			('vscode', 'vscode'),
			('vscode_insiders', 'vscode'),
			('vscode_web', 'vscode'),
			('code_server', 'vscode'),
			('cursor', 'vscode'),
			('windsurf', 'vscode'),
			('positron', 'vscode'),
			('vscodium', 'vscode'),
			('codium', 'vscode'),
			('antigravity', 'vscode'),
			('trae', 'vscode'),
			('kiro', 'vscode'),
			('devin', 'vscode'),
			('jetbrains', 'jetbrains')
		) AS registry(app, family)
		WHERE registry.app = connection_logs.app_name_or_port
	), kind),
	app_name_or_port = NULL
WHERE kind IN ('ssh', 'reconnecting_pty');

COMMENT ON COLUMN connection_logs.app_name_or_port IS 'Null for SSH events. For web connections, this is the slug of the app or the port number being forwarded.';

COMMENT ON COLUMN connection_logs.kind IS NULL;

ALTER TABLE connection_logs
	RENAME COLUMN app_name_or_port TO slug_or_port;

ALTER TABLE connection_logs
	RENAME COLUMN kind TO type;

ALTER TYPE connection_kind RENAME TO connection_type;
