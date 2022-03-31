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
	workspace_resources (
		id,
		created_at,
		job_id,
		transition,
		address,
		type,
		name,
		agent_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;
