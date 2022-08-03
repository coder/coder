-- name: GetTemplateByID :one
SELECT
	*
FROM
	templates
WHERE
	id = $1
LIMIT
	1;

-- name: GetTemplatesWithFilter :many
SELECT
	*
FROM
	templates
WHERE
	-- Optionally include deleted templates
	templates.deleted = @deleted
	-- Filter by organization_id
	AND CASE
		WHEN @organization_id :: uuid != '00000000-00000000-00000000-00000000' THEN
			organization_id = @organization_id
		ELSE true
	END
	-- Filter by exact name
	AND CASE
		WHEN @exact_name :: text != '' THEN
			LOWER("name") = LOWER(@exact_name)
		ELSE true
	END
	-- Filter by ids
	AND CASE
		WHEN array_length(@ids :: uuid[], 1) > 0 THEN
			id = ANY(@ids)
		ELSE true
	END
ORDER BY (created_at, id) ASC
;

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

-- name: GetTemplates :many
SELECT * FROM templates
ORDER BY (created_at, id) ASC
;

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
		created_by
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: UpdateTemplateActiveVersionByID :exec
UPDATE
	templates
SET
	active_version_id = $2,
	updated_at = $3
WHERE
	id = $1;

-- name: UpdateTemplateDeletedByID :exec
UPDATE
	templates
SET
	deleted = $2,
	updated_at = $3
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
