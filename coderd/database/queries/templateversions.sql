-- name: GetTemplateVersionsByTemplateID :many
SELECT
	*
FROM
	template_version_with_user AS template_versions
WHERE
	template_id = @template_id :: uuid
	AND CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
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
	-- A null limit means "no limit", so 0 means return all
	NULLIF(@limit_opt :: int, 0);

-- name: GetTemplateVersionByJobID :one
SELECT
	*
FROM
	template_version_with_user AS template_versions
WHERE
	job_id = $1;

-- name: GetTemplateVersionsCreatedAfter :many
SELECT * FROM template_version_with_user AS template_versions WHERE created_at > $1;

-- name: GetTemplateVersionByTemplateIDAndName :one
SELECT
	*
FROM
	template_version_with_user AS template_versions
WHERE
	template_id = $1
	AND "name" = $2;

-- name: GetTemplateVersionByID :one
SELECT
	*
FROM
	template_version_with_user AS template_versions
WHERE
	id = $1;

-- name: GetTemplateVersionsByIDs :many
SELECT
	*
FROM
	template_version_with_user AS template_versions
WHERE
	id = ANY(@ids :: uuid [ ]);

-- name: InsertTemplateVersion :exec
INSERT INTO
	template_versions (
		id,
		template_id,
		organization_id,
		created_at,
		updated_at,
		"name",
		message,
		readme,
		job_id,
		created_by
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpdateTemplateVersionByID :exec
UPDATE
	template_versions
SET
	template_id = $2,
	updated_at = $3,
	name = $4,
	message = $5
WHERE
	id = $1;

-- name: UpdateTemplateVersionDescriptionByJobID :exec
UPDATE
	template_versions
SET
	readme = $2,
	updated_at = $3
WHERE
	job_id = $1;

-- name: UpdateTemplateVersionExternalAuthProvidersByJobID :exec
UPDATE
	template_versions
SET
	external_auth_providers = $2,
	updated_at = $3
WHERE
	job_id = $1;

-- name: GetPreviousTemplateVersion :one
SELECT
	*
FROM
	template_version_with_user AS template_versions
WHERE
	created_at < (
		SELECT created_at
		FROM template_version_with_user AS tv
		WHERE tv.organization_id = $1 AND tv.name = $2 AND tv.template_id = $3
	)
	AND organization_id = $1
	AND template_id = $3
ORDER BY created_at DESC
LIMIT 1;
