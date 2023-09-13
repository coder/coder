-- name: InsertWorkspaceAgentScripts :many
INSERT INTO
	workspace_agent_scripts (workspace_agent_id, created_at, log_source_id, log_path, script, cron, start_blocks_login, run_on_start, run_on_stop, timeout_seconds)
SELECT
	@workspace_agent_id :: uuid AS workspace_agent_id,
	@created_at :: timestamptz AS created_at,
	unnest(@log_source_id :: uuid [ ]) AS log_source_id,
	unnest(@log_path :: text [ ]) AS log_path,
	unnest(@script :: text [ ]) AS script,
	unnest(@cron :: text [ ]) AS cron,
	unnest(@start_blocks_login :: boolean [ ]) AS start_blocks_login,
	unnest(@run_on_start :: boolean [ ]) AS run_on_start,
	unnest(@run_on_stop :: boolean [ ]) AS run_on_stop,
	unnest(@timeout_seconds :: integer [ ]) AS timeout_seconds
RETURNING workspace_agent_scripts.*;

-- name: GetWorkspaceAgentScriptsByAgentIDs :many
SELECT * FROM workspace_agent_scripts WHERE workspace_agent_id = ANY(@ids :: uuid [ ]);
