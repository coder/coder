-- name: GetWorkspaces :many
SELECT * FROM workspaces WHERE deleted = $1;

-- name: GetWorkspaceByID :one
SELECT
	*
FROM
	workspaces
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspacesWithFilter :many
SELECT
    *
FROM
    workspaces
WHERE
    -- Optionally include deleted workspaces
	deleted = @deleted
	-- Filter by organization_id
	AND CASE
		WHEN @organization_id :: uuid != '00000000-00000000-00000000-00000000' THEN
			organization_id = @organization_id
		ELSE true
	END
	-- Filter by owner_id
	AND CASE
		  WHEN @owner_id :: uuid != '00000000-00000000-00000000-00000000' THEN
				owner_id = @owner_id
		  ELSE true
	END
;

-- name: GetWorkspacesByOrganizationIDs :many
SELECT * FROM workspaces WHERE organization_id = ANY(@ids :: uuid [ ]) AND deleted = @deleted;

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

-- name: GetWorkspacesByTemplateID :many
SELECT
	*
FROM
	workspaces
WHERE
	template_id = $1
	AND deleted = $2;

-- name: GetWorkspaceByOwnerIDAndName :one
SELECT
	*
FROM
	workspaces
WHERE
	owner_id = @owner_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name);

-- name: GetWorkspaceOwnerCountsByTemplateIDs :many
SELECT
	template_id,
	COUNT(DISTINCT owner_id)
FROM
	workspaces
WHERE
	template_id = ANY(@ids :: uuid [ ])
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
