-- name: InsertWorkspaceAgentTask :one
INSERT INTO workspace_agent_tasks (
    id,
    agent_id,
    created_at,
    reporter,
    summary,
    url,
    icon,
    completion
) VALUES (
    @id,
    @agent_id,
    @created_at,
    @reporter,
    @summary,
    @url,
    @icon,
    @completion
) RETURNING *;


-- name: GetWorkspaceAgentTasksByAgentIDs :many
SELECT * FROM workspace_agent_tasks
WHERE agent_id = ANY(@ids::uuid[])
ORDER BY created_at DESC;


-- name: UpdateWorkspaceAgentTask :exec
UPDATE workspace_agents SET
    task_waiting_for_user_input = @task_waiting_for_user_input,
    task_completed_at = @task_completed_at,
    task_notifications = @task_notifications
WHERE id = @id;
