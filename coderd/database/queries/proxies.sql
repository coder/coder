-- name: InsertWorkspaceProxy :one
INSERT INTO
	workspace_proxies (
		id,
		name,
		display_name,
		icon,
		url,
		wildcard_hostname,
		token_hashed_secret,
		created_at,
		updated_at,
		deleted
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, false) RETURNING *;

-- name: UpdateWorkspaceProxy :one
UPDATE
	workspace_proxies
SET
	name = @name,
	display_name = @display_name,
	url = @url,
	wildcard_hostname = @wildcard_hostname,
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

-- Finds a workspace proxy that has an access URL or app hostname that matches
-- the provided hostname. This is to check if a hostname matches any workspace
-- proxy.
--
-- The hostname must be sanitized to only contain [a-zA-Z0-9.-] before calling
-- this query. The scheme, port and path should be stripped.
--
-- name: GetWorkspaceProxyByHostname :one
SELECT
	*
FROM
	workspace_proxies
WHERE
	-- Validate that the @hostname has been sanitized and is not empty. This
	-- doesn't prevent SQL injection (already prevented by using prepared
	-- queries), but it does prevent carefully crafted hostnames from matching
	-- when they shouldn't.
	--
	-- Periods don't need to be escaped because they're not special characters
	-- in SQL matches unlike regular expressions.
	@hostname :: text SIMILAR TO '[a-zA-Z0-9.-]+' AND
	deleted = false AND

	-- Validate that the hostname matches either the wildcard hostname or the
	-- access URL (ignoring scheme, port and path).
	(
		url SIMILAR TO '[^:]*://' || @hostname :: text || '([:/]?%)*' OR
		@hostname :: text LIKE replace(wildcard_hostname, '*', '%')
	)
LIMIT
	1;

-- name: GetWorkspaceProxies :many
SELECT
	*
FROM
	workspace_proxies
WHERE
	deleted = false;
