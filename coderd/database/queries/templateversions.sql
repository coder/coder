-- name: GetTemplateVersionsByTemplateID :many
SELECT
	*
FROM
	template_versions
WHERE
	template_id = @template_id :: uuid
	AND CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @after_id :: uuid != '00000000-00000000-00000000-00000000' THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the created_at field, so select all
			-- rows after the cursor.
			(created_at, id) > (
				SELECT
					created_at, id
				FROM
					template_versions
				WHERE
					id = @after_id
			)
		)
		ELSE true
	END
ORDER BY
    -- Deterministic and consistent ordering of all rows, even if they share
    -- a timestamp. This is to ensure consistent pagination.
	(created_at, id) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so -1 means return all
	NULLIF(@limit_opt :: int, -1);

-- name: GetTemplateVersionByJobID :one
SELECT
	*
FROM
	template_versions
WHERE
	job_id = $1;

-- name: GetTemplateVersionsCreatedAfter :many
SELECT * FROM template_versions WHERE created_at > $1;

-- name: GetTemplateVersionByTemplateIDAndName :one
SELECT
	*
FROM
	template_versions
WHERE
	template_id = $1
	AND "name" = $2;

-- name: GetTemplateVersionByID :one
SELECT
	*
FROM
	template_versions
WHERE
	id = $1;

-- name: InsertTemplateVersion :one
INSERT INTO
	template_versions (
		id,
		template_id,
		organization_id,
		created_at,
		updated_at,
		"name",
		readme,
		job_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateTemplateVersionByID :exec
UPDATE
	template_versions
SET
	template_id = $2,
	updated_at = $3
WHERE
	id = $1;

-- name: UpdateTemplateVersionDescriptionByJobID :exec
UPDATE
	template_versions
SET
	readme = $2,
	updated_at = now()
WHERE
	job_id = $1;
