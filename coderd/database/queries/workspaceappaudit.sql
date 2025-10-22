-- name: UpsertWorkspaceAppAuditSession :one
--
-- The returned boolean, new_or_stale, can be used to deduce if a new session
-- was started. This means that a new row was inserted (no previous session) or
-- the updated_at is older than stale interval.
INSERT INTO
	workspace_app_audit_sessions (
		id,
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
		$9,
		$10
	)
ON CONFLICT
	(agent_id, app_id, user_id, ip, user_agent, slug_or_port, status_code)
DO
	UPDATE
	SET
		-- ID is used to know if session was reset on upsert.
		id = CASE
			WHEN workspace_app_audit_sessions.updated_at > NOW() - (@stale_interval_ms::bigint || ' ms')::interval
			THEN workspace_app_audit_sessions.id
			ELSE EXCLUDED.id
		END,
		started_at = CASE
			WHEN workspace_app_audit_sessions.updated_at > NOW() - (@stale_interval_ms::bigint || ' ms')::interval
			THEN workspace_app_audit_sessions.started_at
			ELSE EXCLUDED.started_at
		END,
		updated_at = EXCLUDED.updated_at
RETURNING
	id = $1 AS new_or_stale;
