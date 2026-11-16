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
	workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, instance_type, daily_cost, module_path)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: GetWorkspaceResourceMetadataByResourceIDs :many
SELECT
	*
FROM
	workspace_resource_metadata
WHERE
	workspace_resource_id = ANY(@ids :: uuid [ ]) ORDER BY id ASC;

-- name: InsertWorkspaceResourceMetadata :many
INSERT INTO
	workspace_resource_metadata (workspace_resource_id, key, value, sensitive, value_key_id)
SELECT
	@workspace_resource_id :: uuid AS workspace_resource_id,
	unnest(@key :: text [ ]) AS key,
	unnest(@value :: text [ ]) AS value,
	unnest(@sensitive :: boolean [ ]) AS sensitive,
	-- Empty string means not encrypted; the column has a foreign key to
	-- dbcrypt_keys, so it must be NULL rather than ''.
	NULLIF(unnest(@value_key_id :: text [ ]), '') AS value_key_id
RETURNING *;

-- name: UpdateWorkspaceResourceMetadatumValue :exec
-- Rewrites a metadata value in place. Used by `coder server dbcrypt`
-- maintenance commands to re-encrypt or decrypt sensitive values at rest.
UPDATE
	workspace_resource_metadata
SET
	value = sqlc.narg('value'),
	value_key_id = sqlc.narg('value_key_id')
WHERE
	id = @id;

-- name: GetWorkspaceResourceMetadataCreatedAfter :many
SELECT * FROM workspace_resource_metadata WHERE workspace_resource_id = ANY(
	SELECT id FROM workspace_resources WHERE created_at > $1
);
