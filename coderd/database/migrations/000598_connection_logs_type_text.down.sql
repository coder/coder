CREATE TYPE connection_type AS ENUM (
	'ssh',
	'vscode',
	'jetbrains',
	'reconnecting_pty',
	'workspace_app',
	'port_forwarding',
	'tunnel'
);

-- Fold app names into the families the enum holds. SQL cannot call into Go,
-- so this copies codersdk.sessionApps; a name it does not know counts as ssh
-- rather than losing the row.
UPDATE connection_logs
SET type = COALESCE((
	SELECT registry.family
	FROM (VALUES
		('zed', 'ssh'),
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
		('devin', 'vscode')
	) AS registry(app, family)
	WHERE registry.app = connection_logs.type
), 'ssh')
WHERE type NOT IN (
	'ssh', 'vscode', 'jetbrains', 'reconnecting_pty',
	'workspace_app', 'port_forwarding', 'tunnel'
);

ALTER TABLE connection_logs
	ALTER COLUMN type TYPE connection_type USING type::connection_type;
