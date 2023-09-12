-- name: ActivityBumpWorkspace :exec
WITH latest AS (
	SELECT
		workspace_builds.id::uuid AS build_id,
		workspace_builds.deadline::timestamp AS build_deadline,
		workspace_builds.max_deadline::timestamp AS build_max_deadline,
		workspace_builds.transition AS build_transition,
		provisioner_jobs.completed_at::timestamp AS job_completed_at,
		(workspaces.ttl / 1000 / 1000 / 1000 || ' seconds')::interval AS ttl_interval,
		(NOW() AT TIME ZONE 'UTC')::timestamp as now_utc
	FROM workspace_builds
	JOIN provisioner_jobs
		ON provisioner_jobs.id = workspace_builds.job_id
	JOIN workspaces
		ON workspaces.id = workspace_builds.workspace_id
	WHERE workspace_builds.workspace_id = $1::uuid
	ORDER BY workspace_builds.build_number DESC
	LIMIT 1
)
UPDATE
	workspace_builds wb
SET
	updated_at = NOW(),
	deadline = CASE
		WHEN l.build_max_deadline = '0001-01-01 00:00:00'
		THEN l.now_utc + l.ttl_interval
		ELSE LEAST(l.now_utc + l.ttl_interval, l.build_max_deadline)
	END
FROM latest l
WHERE wb.id = l.build_id
AND l.build_transition = 'start'
AND l.build_deadline != '0001-01-01 00:00:00'
AND l.job_completed_at IS NOT NULL
AND l.build_deadline + (l.ttl_interval * 0.05) < NOW()
;
