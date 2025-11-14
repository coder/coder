-- name: UpsertWorkspaceAppAuditSession :one
--
-- The returned boolean, new_or_stale, can be used to deduce if a new session
-- was started. This means that a new row was inserted (no previous session) or
-- the updated_at is older than stale interval.
WITH filtered_input AS (
	-- By filtering the input this way, we can avoid upsert write operations
	-- (which PostgreSQL does not optimize away even if the input is the same)
	-- on a specified interval (1 minute). This means we lose some precision
	-- but avoid unnecessary writes.
	SELECT
		@id::uuid AS id,
		@agent_id::uuid AS agent_id,
		@app_id::uuid AS app_id,
		@user_id::uuid AS user_id,
		@ip::text AS ip,
		@user_agent::text AS user_agent,
		@slug_or_port::text AS slug_or_port,
		@status_code::int4 AS status_code,
		@started_at::timestamptz AS started_at,
		@updated_at::timestamptz AS updated_at
	WHERE
		NOT EXISTS (
			SELECT 1
			FROM workspace_app_audit_sessions w
			WHERE
				w.agent_id = @agent_id
				AND w.app_id = @app_id
				AND w.user_id = @user_id
				AND w.ip = @ip
				AND w.user_agent = @user_agent
				AND w.slug_or_port = @slug_or_port
				AND w.status_code = @status_code
				AND w.updated_at >= NOW() - '1 minute'::interval
		)
),
upsert_result AS (
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
	SELECT
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
	FROM
		filtered_input
	ON CONFLICT
		(agent_id, app_id, user_id, ip, user_agent, slug_or_port, status_code)
	DO UPDATE
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
		id
)
SELECT
	EXISTS(SELECT 1 FROM upsert_result WHERE id = @id) AS new_or_stale;
