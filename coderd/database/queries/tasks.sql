-- name: InsertTask :one
INSERT INTO tasks
	(id, organization_id, owner_id, name, workspace_id, template_version_id, template_parameters, prompt, created_at)
VALUES
	(gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpsertTaskWorkspaceApp :one
INSERT INTO task_workspace_apps
	(task_id, workspace_build_number, workspace_agent_id, workspace_app_id)
VALUES
	($1, $2, $3, $4)
ON CONFLICT (task_id, workspace_build_number)
DO UPDATE SET
	workspace_agent_id = EXCLUDED.workspace_agent_id,
	workspace_app_id = EXCLUDED.workspace_app_id
RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks_with_status WHERE id = @id::uuid;

-- name: GetTaskByWorkspaceID :one
SELECT * FROM tasks_with_status WHERE workspace_id = @workspace_id::uuid;

-- name: ListTasks :many
SELECT * FROM tasks_with_status tws
WHERE tws.deleted_at IS NULL
AND CASE WHEN @owner_id::UUID != '00000000-0000-0000-0000-000000000000' THEN tws.owner_id = @owner_id::UUID ELSE TRUE END
AND CASE WHEN @organization_id::UUID != '00000000-0000-0000-0000-000000000000' THEN tws.organization_id = @organization_id::UUID ELSE TRUE END
ORDER BY tws.created_at DESC;
