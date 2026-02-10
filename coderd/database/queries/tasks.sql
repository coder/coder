-- name: InsertTask :one
INSERT INTO tasks
	(id, organization_id, owner_id, name, display_name, workspace_id, template_version_id, template_parameters, prompt, created_at)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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

-- name: GetTaskByOwnerIDAndName :one
SELECT * FROM tasks_with_status
WHERE
	owner_id = @owner_id::uuid
	AND deleted_at IS NULL
	AND LOWER(name) = LOWER(@name::text);

-- name: ListTasks :many
SELECT * FROM tasks_with_status tws
WHERE tws.deleted_at IS NULL
AND CASE WHEN @owner_id::UUID != '00000000-0000-0000-0000-000000000000' THEN tws.owner_id = @owner_id::UUID ELSE TRUE END
AND CASE WHEN @organization_id::UUID != '00000000-0000-0000-0000-000000000000' THEN tws.organization_id = @organization_id::UUID ELSE TRUE END
AND CASE WHEN @status::text != '' THEN tws.status = @status::task_status ELSE TRUE END
ORDER BY tws.created_at DESC;

-- name: DeleteTask :one
WITH deleted_task AS (
	UPDATE tasks
	SET
		deleted_at = @deleted_at::timestamptz
	WHERE
		id = @id::uuid
		AND deleted_at IS NULL
	RETURNING id
), deleted_snapshot AS (
	DELETE FROM task_snapshots
	WHERE task_id = @id::uuid
)
SELECT id FROM deleted_task;


-- name: UpdateTaskPrompt :one
UPDATE
	tasks
SET
	prompt = @prompt::text
WHERE
	id = @id::uuid
	AND deleted_at IS NULL
RETURNING *;

-- name: UpsertTaskSnapshot :exec
INSERT INTO
	task_snapshots (task_id, log_snapshot, log_snapshot_created_at)
VALUES
	($1, $2, $3)
ON CONFLICT
	(task_id)
DO UPDATE SET
	log_snapshot = EXCLUDED.log_snapshot,
	log_snapshot_created_at = EXCLUDED.log_snapshot_created_at;

-- name: GetTaskSnapshot :one
SELECT
	*
FROM
	task_snapshots
WHERE
	task_id = $1;

-- name: GetTasksForTelemetry :many
-- Returns tasks with their workspace app bindings for telemetry collection.
-- This bypasses the expensive tasks_with_status view by querying the base
-- tables directly.
SELECT
    t.id,
    t.organization_id,
    t.owner_id,
    t.name,
    t.workspace_id,
    t.template_version_id,
    t.prompt,
    t.created_at,
    -- COALESCE to 0 because sqlc cannot infer nullability from a LEFT
    -- JOIN LATERAL. Build numbers start at 1, so 0 means "no binding".
    COALESCE(twa.workspace_build_number, 0) AS workspace_build_number,
    twa.workspace_agent_id,
    twa.workspace_app_id
FROM tasks t
LEFT JOIN LATERAL (
    SELECT
        task_app.workspace_build_number,
        task_app.workspace_agent_id,
        task_app.workspace_app_id
    FROM task_workspace_apps task_app
    WHERE task_app.task_id = t.id
    ORDER BY task_app.workspace_build_number DESC
    LIMIT 1
) twa ON TRUE
WHERE t.deleted_at IS NULL
ORDER BY t.created_at DESC;
