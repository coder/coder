-- name: InsertTask :one
INSERT INTO tasks
	(id, organization_id, owner_id, name, workspace_id, template_version_id, template_parameters, prompt, created_at)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateTaskWorkspaceID :one
UPDATE
	tasks
SET
	workspace_id = $2
FROM
	workspaces w
JOIN
	template_versions tv
ON
	tv.template_id = w.template_id
WHERE
	tasks.id = $1
	AND tasks.workspace_id IS NULL
	AND w.id = $2
	AND tv.id = tasks.template_version_id
RETURNING
	tasks.*;

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
AND CASE WHEN @status::text != '' THEN tws.status = @status::task_status ELSE TRUE END
ORDER BY tws.created_at DESC;

-- name: DeleteTask :one
UPDATE tasks
SET
	deleted_at = @deleted_at::timestamptz
WHERE
	id = @id::uuid
	AND deleted_at IS NULL
RETURNING *;
