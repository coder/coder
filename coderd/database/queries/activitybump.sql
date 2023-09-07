-- name: ActivityBumpWorkspace :exec
UPDATE
	workspace_builds
SET
	updated_at = $2,
	deadline = LEAST(workspace_builds.deadline + workspace.ttl, workspace_builds.max_deadline)
WHERE
	workspace_builds.id IN (
		SELECT wb.id
		FROM workspace_builds wb
		JOIN provisioner_jobs pj
			ON pj.id = wb.job_id
		WHERE wb.workspace_id = $1
		AND wb.transition == 'start'
		AND wb.deadline > $2
		AND wb.deadline != wb.max_deadline
		AND pj.completed_at IS NOT NULL
	ORDER BY wb.build_number ASC
	LIMIT 1
);
