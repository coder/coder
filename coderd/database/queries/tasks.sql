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

-- name: GetTelemetryTaskEvents :many
-- Returns all data needed to build task lifecycle events for telemetry
-- in a single round-trip. For each task whose workspace is in the
-- given set, fetches:
--   - the latest workspace app binding (task_workspace_apps)
--   - the most recent stop and start builds (workspace_builds)
--   - the last "working" app status (workspace_app_statuses)
--   - the first app status after resume, for active workspaces
--
-- Assumptions:
-- - 1:1 relationship between tasks and workspaces. All builds on the
--   workspace are considered task-related.
-- - Idle duration approximation: If the agent reports "working", does
--   work, then reports "done", we miss that working time.
WITH task_event_data AS (
    SELECT
        t.id AS task_id,
        t.workspace_id,
        twa.workspace_app_id,
        -- Latest stop build.
        stop_build.created_at AS stop_build_created_at,
        stop_build.reason AS stop_build_reason,
        -- Latest start build (task_resume only).
        start_build.created_at AS start_build_created_at,
        -- Last "working" app status (for idle duration).
        lws.created_at AS last_working_status_at,
        -- First app status after resume (for resume-to-status duration).
        -- Only populated for workspaces in an active phase (started more
        -- recently than stopped).
        fsar.created_at AS first_status_after_resume_at,
        -- Cumulative time spent in "working" state.
        active_dur.total_working_ms AS active_duration_ms
    FROM tasks t
    LEFT JOIN LATERAL (
        SELECT task_app.workspace_app_id
        FROM task_workspace_apps task_app
        WHERE task_app.task_id = t.id
        ORDER BY task_app.workspace_build_number DESC
        LIMIT 1
    ) twa ON TRUE
    LEFT JOIN LATERAL (
        SELECT wb.created_at, wb.reason
        FROM workspace_builds wb
        WHERE wb.workspace_id = t.workspace_id
            AND wb.transition = 'stop'
        ORDER BY wb.build_number DESC
        LIMIT 1
    ) stop_build ON TRUE
    LEFT JOIN LATERAL (
        SELECT wb.created_at
        FROM workspace_builds wb
        WHERE wb.workspace_id = t.workspace_id
            AND wb.transition = 'start'
            AND wb.reason = 'task_resume'
        ORDER BY wb.build_number DESC
        LIMIT 1
    ) start_build ON TRUE
    LEFT JOIN LATERAL (
        SELECT was.created_at
        FROM workspace_app_statuses was
        WHERE was.app_id = twa.workspace_app_id
            AND was.state = 'working'
        ORDER BY was.created_at DESC
        LIMIT 1
    ) lws ON twa.workspace_app_id IS NOT NULL
    LEFT JOIN LATERAL (
        SELECT was.created_at
        FROM workspace_app_statuses was
        WHERE was.app_id = twa.workspace_app_id
            AND was.created_at > start_build.created_at
        ORDER BY was.created_at ASC
        LIMIT 1
    ) fsar ON twa.workspace_app_id IS NOT NULL
        AND start_build.created_at IS NOT NULL
        AND (stop_build.created_at IS NULL
            OR start_build.created_at > stop_build.created_at)
    -- Active duration: cumulative time spent in "working" state.
    -- Uses LEAD() to convert point-in-time statuses into intervals, then sums
    -- intervals where state='working'. For the last status, falls back to
    -- stop_build time (if paused) or @now (if still running).
    LEFT JOIN LATERAL (
        SELECT COALESCE(
            SUM(EXTRACT(EPOCH FROM (interval_end - interval_start)) * 1000)::bigint,
            0
        ) AS total_working_ms
        FROM (
            SELECT
                was.created_at AS interval_start,
                COALESCE(
                    LEAD(was.created_at) OVER (ORDER BY was.created_at ASC),
                    COALESCE(stop_build.created_at, @now::timestamptz)
                ) AS interval_end,
                was.state
            FROM workspace_app_statuses was
            WHERE was.app_id = twa.workspace_app_id
        ) intervals
        WHERE intervals.state = 'working'
    ) active_dur ON twa.workspace_app_id IS NOT NULL
    WHERE t.deleted_at IS NULL
        AND t.workspace_id IS NOT NULL
        AND EXISTS (
            SELECT 1 FROM workspace_builds wb
            WHERE wb.workspace_id = t.workspace_id
              AND wb.created_at > @created_after
        )
)
SELECT * FROM task_event_data
ORDER BY task_id;
