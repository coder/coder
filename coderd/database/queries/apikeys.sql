-- name: GetAPIKeyByID :one
SELECT
	*
FROM
	api_keys
WHERE
	id = $1
LIMIT
	1;

-- name: GetAPIKeysLastUsedAfter :many
SELECT * FROM api_keys WHERE last_used > $1;

-- name: InsertAPIKey :one
INSERT INTO
	api_keys (
		id,
		lifetime_seconds,
		hashed_secret,
		ip_address,
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
	(@id,
	 -- If the lifetime is set to 0, default to 24hrs
	 CASE @lifetime_seconds::bigint
	     WHEN 0 THEN 86400
		 ELSE @lifetime_seconds::bigint
	 END
	 , @hashed_secret, @ip_address, @user_id, @last_used, @expires_at, @created_at, @updated_at, @login_type, @oauth_access_token, @oauth_refresh_token, @oauth_id_token, @oauth_expiry) RETURNING *;

-- name: UpdateAPIKeyByID :exec
UPDATE
	api_keys
SET
	last_used = $2,
	expires_at = $3,
	ip_address = $4,
	oauth_access_token = $5,
	oauth_refresh_token = $6,
	oauth_expiry = $7
WHERE
	id = $1;

-- name: DeleteAPIKeyByID :exec
DELETE
FROM
	api_keys
WHERE
	id = $1;
