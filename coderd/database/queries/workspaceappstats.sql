-- name: InsertWorkspaceAppStats :exec
INSERT INTO
	workspace_app_stats (
		user_id,
		workspace_id,
		agent_id,
		access_method,
		slug_or_port,
		session_id,
		session_started_at,
		session_ended_at,
		requests
	)
SELECT
	unnest(@user_id::uuid[]) AS user_id,
	unnest(@workspace_id::uuid[]) AS workspace_id,
	unnest(@agent_id::uuid[]) AS agent_id,
	unnest(@access_method::text[]) AS access_method,
	unnest(@slug_or_port::text[]) AS slug_or_port,
	unnest(@session_id::uuid[]) AS session_id,
	unnest(@session_started_at::timestamptz[]) AS session_started_at,
	unnest(@session_ended_at::timestamptz[]) AS session_ended_at,
	unnest(@requests::int[]) AS requests
ON CONFLICT
	(user_id, agent_id, session_id)
DO
	UPDATE SET
		session_ended_at = EXCLUDED.session_ended_at,
		requests = EXCLUDED.requests
	WHERE
		workspace_app_stats.user_id = EXCLUDED.user_id
		AND workspace_app_stats.agent_id = EXCLUDED.agent_id
		AND workspace_app_stats.session_id = EXCLUDED.session_id
		-- Since stats are updated in place as time progresses, we only
		-- want to update this row if it's fresh.
		AND workspace_app_stats.session_ended_at <= EXCLUDED.session_ended_at
		AND workspace_app_stats.requests <= EXCLUDED.requests;
