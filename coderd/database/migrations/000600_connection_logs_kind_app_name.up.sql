-- Renames only, so the table is not rewritten.
ALTER TYPE connection_type RENAME TO connection_kind;

ALTER TABLE connection_logs
	RENAME COLUMN type TO kind;

ALTER TABLE connection_logs
	RENAME COLUMN slug_or_port TO app_name_or_port;

COMMENT ON COLUMN connection_logs.kind IS 'What observed the connection: the agent (ssh, reconnecting_pty) or coderd (workspace_app, port_forwarding, tunnel). vscode and jetbrains appear only on older rows, as ssh by that app.';

COMMENT ON COLUMN connection_logs.app_name_or_port IS 'By kind: the app the agent reported, the workspace app slug, or the forwarded port. Null for tunnels and for older agent rows, whose kind names the app.';
