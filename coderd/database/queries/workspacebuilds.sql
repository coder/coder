-- name: InsertWorkspaceBuild :one
INSERT INTO
	workspace_builds (
		id,
		created_at,
		updated_at,
		workspace_id,
		template_version_id,
		"build_number",
		transition,
		initiator_id,
		job_id,
		provisioner_state,
		deadline,
		reason
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateWorkspaceBuildByID :one
UPDATE
	workspace_builds
SET
	updated_at = $2,
	provisioner_state = $3,
	deadline = $4
WHERE
	id = $1 RETURNING *;

-- name: UpdateWorkspaceBuildCostByID :one
UPDATE
	workspace_builds
SET
	daily_cost = $2
WHERE
	id = $1 RETURNING *;

