-- name: GetWorkspaceResourceByID :one
SELECT
	*
FROM
	workspace_resources
WHERE
	id = $1;

-- name: GetWorkspaceResourcesByJobID :many
SELECT
	*
FROM
	workspace_resources
WHERE
	job_id = $1;

-- name: InsertWorkspaceResource :one
INSERT INTO
	workspace_resources (id, created_at, job_id, transition, type, name)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetWorkspaceResources :many
SELECT
	workspace_resources.*
FROM
	workspace_resources
INNER JOIN workspace_builds
	ON workspace_resources.job_id = workspace_builds.job_id
WHERE
  workspace_builds.after_id IS NULL;
