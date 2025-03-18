-- name: UpsertWorkspaceAppAuditSession :one
--
-- Insert a new workspace app audit session or update an existing one, if
-- started_at is updated, it means the session has been restarted.
INSERT INTO
	workspace_app_audit_sessions (
		agent_id,
		app_id,
		user_id,
		ip,
		user_agent,
		slug_or_port,
		status_code,
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
		$7,
		$8,
		$9
	)
ON CONFLICT
	(agent_id, app_id, user_id, ip, user_agent, slug_or_port, status_code)
DO
	UPDATE
	SET
		started_at = CASE
			WHEN workspace_app_audit_sessions.updated_at > NOW() - (@stale_interval_ms::bigint || ' ms')::interval
			THEN workspace_app_audit_sessions.started_at
			ELSE EXCLUDED.started_at
		END,
		updated_at = EXCLUDED.updated_at
RETURNING
	started_at;
