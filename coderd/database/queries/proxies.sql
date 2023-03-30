-- name: InsertWorkspaceProxy :one
INSERT INTO
	workspace_proxies (
		id,
		organization_id,
		name,
		icon,
		url,
		wildcard_url,
		created_at,
		updated_at
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateWorkspaceProxy :one
UPDATE
	workspace_proxies
SET
	name = @name,
	url = @url,
	wildcard_url = @wildcard_url,
	icon = @icon,
	updated_at = Now()
WHERE
	id = @id
RETURNING *;


-- name: UpdateWorkspaceProxyDeleted :exec
UPDATE
	workspace_proxies
SET
    updated_at = Now(),
	deleted = @deleted
WHERE
	id = @id;

-- name: GetWorkspaceProxyByID :one
SELECT
	*
FROM
	workspace_proxies
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspaceProxies :many
SELECT
	*
FROM
	workspace_proxies
WHERE
	deleted = false;
