-- name: InsertTask :one
INSERT INTO tasks
	(id, organization_id, owner_id, name, workspace_id, template_version_id, template_parameters, prompt, created_at)
VALUES
	(gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: InsertTaskWorkspaceApp :one
INSERT INTO task_workspace_apps
	(task_id, workspace_build_id, workspace_agent_id, workspace_app_id)
VALUES
	($1, $2, $3, $4)
RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks_with_status WHERE id = $1;
