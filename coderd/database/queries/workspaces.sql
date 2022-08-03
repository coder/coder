-- name: GetWorkspaceByID :one
SELECT
	*
FROM
	workspaces
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspaces :many
SELECT
    *
FROM
    workspaces
WHERE
    -- Optionally include deleted workspaces
	workspaces.deleted = @deleted
	-- Filter by owner_id
	AND CASE
		WHEN @owner_id :: uuid != '00000000-00000000-00000000-00000000' THEN
			owner_id = @owner_id
		ELSE true
	END
  	-- Filter by owner_name
	AND CASE
		WHEN @owner_username :: text != '' THEN
			owner_id = (SELECT id FROM users WHERE lower(username) = lower(@owner_username))
		ELSE true
	END
	-- Filter by template_name
	-- There can be more than 1 template with the same name across organizations.
  	-- Use the organization filter to restrict to 1 org if needed.
	AND CASE
		WHEN @template_name :: text != '' THEN
			template_id = ANY(SELECT id FROM templates WHERE lower(name) = lower(@template_name))
		ELSE true
	END
	-- Filter by template_ids
	AND CASE
		WHEN array_length(@template_ids :: uuid[], 1) > 0 THEN
			template_id = ANY(@template_ids)
		ELSE true
	END
	-- Filter by name, matching on substring
	AND CASE
		WHEN @name :: text != '' THEN
		    name ILIKE '%' || @name || '%'
		ELSE true
	END
;

-- name: GetWorkspacesAutostart :many
SELECT
	*
FROM
	workspaces
WHERE
	deleted = false
AND
(
	(autostart_schedule IS NOT NULL AND autostart_schedule <> '')
	OR
	(ttl IS NOT NULL AND ttl > 0)
);

-- name: GetWorkspaceByOwnerIDAndName :one
SELECT
	*
FROM
	workspaces
WHERE
	owner_id = @owner_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
ORDER BY created_at DESC;

-- name: GetWorkspaceOwnerCountsByTemplateIDs :many
SELECT
	template_id,
	COUNT(DISTINCT owner_id)
FROM
	workspaces
WHERE
	template_id = ANY(@ids :: uuid [ ])
	-- Ignore deleted workspaces
	AND deleted != true
GROUP BY
	template_id;

-- name: InsertWorkspace :one
INSERT INTO
	workspaces (
		id,
		created_at,
		updated_at,
		owner_id,
		organization_id,
		template_id,
		name,
		autostart_schedule,
		ttl
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: UpdateWorkspaceDeletedByID :exec
UPDATE
	workspaces
SET
	deleted = $2
WHERE
	id = $1;

-- name: UpdateWorkspace :exec
UPDATE
	workspaces
SET
	name = $2
WHERE
	id = $1
	AND deleted = false;

-- name: UpdateWorkspaceAutostart :exec
UPDATE
	workspaces
SET
	autostart_schedule = $2
WHERE
	id = $1;

-- name: UpdateWorkspaceTTL :exec
UPDATE
	workspaces
SET
	ttl = $2
WHERE
	id = $1;
