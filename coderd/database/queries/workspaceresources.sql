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

-- name: GetWorkspaceResourcesByJobIDs :many
SELECT
	*
FROM
	workspace_resources
WHERE
	job_id = ANY(@ids :: uuid [ ]);

-- name: GetWorkspaceResourcesCreatedAfter :many
SELECT * FROM workspace_resources WHERE created_at > $1;

-- name: InsertWorkspaceResource :one
INSERT INTO
	workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, instance_type)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetWorkspaceResourceMetadataByResourceID :many
SELECT
	*
FROM
	workspace_resource_metadata
WHERE
	workspace_resource_id = $1;

-- name: GetWorkspaceResourceMetadataByResourceIDs :many
SELECT
	*
FROM
	workspace_resource_metadata
WHERE
	workspace_resource_id = ANY(@ids :: uuid [ ]);

-- name: InsertWorkspaceResourceMetadata :one
INSERT INTO
	workspace_resource_metadata (workspace_resource_id, key, value, sensitive)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: GetWorkspaceResourceMetadataCreatedAfter :many
SELECT * FROM workspace_resource_metadata WHERE workspace_resource_id = ANY(
	SELECT id FROM workspace_resources WHERE created_at > $1
);
