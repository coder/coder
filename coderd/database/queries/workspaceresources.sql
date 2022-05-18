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

-- name: GetLatestWorkspaceResources :many
SELECT workspace_resources.*
FROM (
    SELECT
        workspace_id, MAX(build_number) as max_build_number
    FROM
        workspace_builds
    GROUP BY
        workspace_id
) latest_workspace_builds
INNER JOIN
  workspace_builds
ON
  workspace_builds.workspace_id = latest_workspace_builds.workspace_id
  AND workspace_builds.build_number = latest_workspace_builds.max_build_number
INNER JOIN
  workspace_resources
ON
  workspace_resources.job_id = workspace_builds.job_id;
