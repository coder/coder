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
		application,
		"name",
		last_used,
		expires_at,
		created_at,
		updated_at,
		login_type,
		oidc_access_token,
		oidc_refresh_token,
		oidc_id_token,
		oidc_expiry,
		devurl_token
	)
VALUES
	(
		$1,
		$2,
		$3,
		$4,
		$5,
		$6,
		$7,
		$8,
		$9,
		$10,
		$11,
		$12,
		$13,
		$14,
		$15
	) RETURNING *;

-- name: UpdateAPIKeyByID :exec
UPDATE
	api_keys
SET
	last_used = $2,
	expires_at = $3,
	oidc_access_token = $4,
	oidc_refresh_token = $5,
	oidc_expiry = $6
WHERE
	id = $1;
