-- name: GetAPIKeyByID :one
SELECT
	*
FROM
	api_keys
WHERE
	id = $1
LIMIT
	1;

-- name: InsertAPIKey :one
INSERT INTO
	api_keys (
		id,
		hashed_secret,
		user_id,
		last_used,
		expires_at,
		created_at,
		updated_at,
		login_type,
		oauth_access_token,
		oauth_refresh_token,
		oauth_id_token,
		oauth_expiry
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateAPIKeyByID :exec
UPDATE
	api_keys
SET
	last_used = $2,
	expires_at = $3,
	oauth_access_token = $4,
	oauth_refresh_token = $5,
	oauth_expiry = $6
WHERE
	id = $1;
