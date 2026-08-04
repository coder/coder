-- name: InsertWorkspaceAgentSubagentExecution :one
INSERT INTO workspace_agent_subagent_executions (
	workspace_build_id,
	declaration_id,
	parent_agent_id,
	child_agent_id,
	driver,
	driver_protocol,
	shared_host_path,
	shared_child_path,
	startup_timeout_seconds,
	restart_policy
) VALUES (
	@workspace_build_id,
	@declaration_id,
	@parent_agent_id,
	@child_agent_id,
	@driver,
	@driver_protocol,
	@shared_host_path,
	@shared_child_path,
	@startup_timeout_seconds,
	@restart_policy
)
RETURNING *;

-- name: GetWorkspaceAgentSubagentExecutionsByParentAgentID :many
SELECT *
FROM workspace_agent_subagent_executions
WHERE parent_agent_id = $1
ORDER BY created_at, declaration_id;
