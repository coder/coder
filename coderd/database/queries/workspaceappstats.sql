-- name: InsertWorkspaceAppStats :exec
INSERT INTO
	workspace_app_stats (
		id,
		user_id,
		workspace_id,
		agent_id,
		access_method,
		slug_or_port,
		session_id,
		session_started_at,
		session_ended_at
	)
SELECT
	unnest(@id::uuid[]) AS id,
	unnest(@user_id::uuid[]) AS user_id,
	unnest(@workspace_id::uuid[]) AS workspace_id,
	unnest(@agent_id::uuid[]) AS agent_id,
	unnest(@access_method::text[]) AS access_method,
	unnest(@slug_or_port::text[]) AS slug_or_port,
	unnest(@session_id::uuid[]) AS session_id,
	unnest(@session_started_at::timestamptz[]) AS session_started_at,
	unnest(@session_ended_at::nulltimestamptz[]) AS session_ended_at
ON CONFLICT
	(agent_id, session_id)
DO
	UPDATE SET
		-- Only session end can be updated.
		session_ended_at = EXCLUDED.session_ended_at
	WHERE
		workspace_app_stats.agent_id = EXCLUDED.agent_id
		AND workspace_app_stats.session_id = EXCLUDED.session_id;
