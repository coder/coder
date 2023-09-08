-- name: ActivityBumpWorkspace :exec
WITH latest AS (
	SELECT
		workspace_builds.id,
		workspace_builds.deadline,
		workspace_builds.max_deadline,
		workspaces.ttl,
		(workspace_builds.deadline + (workspaces.ttl/1000 || ' microsecond')::interval ) AS new_deadline
	FROM workspace_builds
	JOIN provisioner_jobs
		ON provisioner_jobs.id = workspace_builds.job_id
	JOIN workspaces
		ON workspaces.id = workspace_builds.workspace_id
	WHERE workspace_builds.workspace_id = $1::uuid
		AND workspace_builds.transition = 'start'
		AND workspace_builds.deadline > NOW()
		AND provisioner_jobs.completed_at IS NOT NULL
	ORDER BY workspace_builds.build_number ASC
	LIMIT 1
)
UPDATE
	workspace_builds wb
SET
	updated_at = NOW(),
	deadline = CASE
		WHEN l.max_deadline = '0001-01-01 00:00:00'
		THEN l.new_deadline
		ELSE LEAST(l.new_deadline, l.max_deadline)
	END
FROM latest l
WHERE
	wb.id = l.id
;
