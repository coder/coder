-- name: GetTemplateByID :one
SELECT
	*
FROM
	templates
WHERE
	id = $1
LIMIT
	1;

-- name: GetTemplatesByIDs :many
SELECT
	*
FROM
	templates
WHERE
	id = ANY(@ids :: uuid [ ]);

-- name: GetTemplateByOrganizationAndName :one
SELECT
	*
FROM
	templates
WHERE
	organization_id = @organization_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
LIMIT
	1;

-- name: GetTemplatesByOrganization :many
SELECT
	*
FROM
	templates
WHERE
	organization_id = $1
	AND deleted = $2;

-- name: InsertTemplate :one
INSERT INTO
	templates (
		id,
		created_at,
		updated_at,
		organization_id,
		"name",
		provisioner,
		active_version_id,
		description,
		max_ttl,
		min_autostart_interval,
		owner_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: UpdateTemplateActiveVersionByID :exec
UPDATE
	templates
SET
	active_version_id = $2
WHERE
	id = $1;

-- name: UpdateTemplateDeletedByID :exec
UPDATE
	templates
SET
	deleted = $2
WHERE
	id = $1;

-- name: UpdateTemplateMetaByID :exec
UPDATE
	templates
SET
	updated_at = $2,
	description = $3,
	max_ttl = $4,
	min_autostart_interval = $5
WHERE
	id = $1
RETURNING
	*;
