-- name: GetWorkspaceBuildByID :one
SELECT
	*
FROM
	workspace_builds
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspaceBuildByJobID :one
SELECT
	*
FROM
	workspace_builds
WHERE
	job_id = $1
LIMIT
	1;

-- name: GetWorkspaceBuildByWorkspaceIDAndName :one
SELECT
	*
FROM
	workspace_builds
WHERE
	workspace_id = $1
	AND "name" = $2;

-- name: GetWorkspaceBuildByWorkspaceID :many
SELECT
	*
FROM
	workspace_builds
WHERE
	workspace_id = $1;

-- name: GetLatestWorkspaceBuildByWorkspaceID :one
SELECT
	*
FROM
	workspace_builds
WHERE
	workspace_id = $1
ORDER BY
    build_number desc
LIMIT
	1;

-- name: GetLatestWorkspaceBuildsByWorkspaceIDs :many
SELECT *, MAX(build_number)
FROM
    workspace_builds
WHERE
    workspace_id = ANY(@ids :: uuid [ ])
GROUP BY
    workspace_id
HAVING
	build_number = MAX(build_number);

-- name: InsertWorkspaceBuild :one
INSERT INTO
	workspace_builds (
		id,
		created_at,
		updated_at,
		workspace_id,
		template_version_id,
		"build_number",
		"name",
		transition,
		initiator_id,
		job_id,
		provisioner_state
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: UpdateWorkspaceBuildByID :exec
UPDATE
	workspace_builds
SET
	updated_at = $2,
	provisioner_state = $3
WHERE
	id = $1;
