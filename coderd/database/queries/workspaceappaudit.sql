-- name: InsertWorkspaceAppAuditSession :one
INSERT INTO
	workspace_app_audit_sessions (
		agent_id,
		app_id,
		user_id,
		ip,
		slug_or_port,
		started_at,
		updated_at
	)
VALUES
	(
		$1,
		$2,
		$3,
		$4,
		$5,
		$6,
		$7
	)
RETURNING
	id;

-- name: UpdateWorkspaceAppAuditSession :many
--
-- Return ID to determine if a row was updated or not. This table isn't strict
-- about uniqueness, so we need to know if we updated an existing row or not.
UPDATE
	workspace_app_audit_sessions
SET
	updated_at = @updated_at
WHERE
	agent_id = @agent_id
	AND app_id IS NOT DISTINCT FROM @app_id
	AND user_id IS NOT DISTINCT FROM @user_id
	AND ip IS NOT DISTINCT FROM @ip
	AND slug_or_port = @slug_or_port
	AND updated_at > NOW() - (@stale_interval_ms::bigint || ' ms')::interval
RETURNING
	id;
