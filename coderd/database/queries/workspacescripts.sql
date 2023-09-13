-- name: InsertWorkspaceAgentScripts :many
INSERT INTO
	workspace_agent_scripts (workspace_agent_id, log_source_id, log_path, created_at, script, cron, start_blocks_login, run_on_start, run_on_stop, timeout)
SELECT
	@workspace_agent_id :: uuid AS workspace_agent_id,
	unnest(@log_source_id :: uuid [ ]) AS log_source_id,
	unnest(@log_path :: text [ ]) AS log_path,
	unnest(@created_at :: timestamptz [ ]) AS created_at,
	unnest(@script :: text [ ]) AS script,
	unnest(@cron :: text [ ]) AS cron,
	unnest(@start_blocks_login :: boolean [ ]) AS start_blocks_login,
	unnest(@run_on_start :: boolean [ ]) AS run_on_start,
	unnest(@run_on_stop :: boolean [ ]) AS run_on_stop,
	unnest(@timeout :: integer [ ]) AS timeout
RETURNING workspace_agent_scripts.*;

-- name: GetWorkspaceAgentScriptsByAgentIDs :many
SELECT * FROM workspace_agent_scripts WHERE workspace_agent_id = ANY(@ids :: uuid [ ]);
