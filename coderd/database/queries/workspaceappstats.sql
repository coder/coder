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
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT
	(agent_id, session_id)
DO
	UPDATE SET
		-- Only session end can be updated.
		session_ended_at = $9
	WHERE
		workspace_app_stats.agent_id = $4
		AND workspace_app_stats.session_id = $7;
