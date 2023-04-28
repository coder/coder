-- name: GetAPIKeyByID :one
SELECT
	*
FROM
	api_keys
WHERE
	id = $1
LIMIT
	1;

-- name: GetAPIKeyByName :one
SELECT
	*
FROM
	api_keys
WHERE
	user_id = @user_id AND
	token_name = @token_name AND
-- there is no unique constraint on empty token names
	token_name != ''
LIMIT
	1;

-- name: GetAPIKeysLastUsedAfter :many
SELECT * FROM api_keys WHERE last_used > $1;

-- name: GetAPIKeysByLoginType :many
SELECT * FROM api_keys WHERE login_type = $1;

-- name: GetAPIKeysByUserID :many
SELECT * FROM api_keys WHERE login_type = $1 AND user_id = $2;

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
		scope,
		token_name
	)
VALUES
	(@id,
	 -- If the lifetime is set to 0, default to 24hrs
	 CASE @lifetime_seconds::bigint
	     WHEN 0 THEN 86400
		 ELSE @lifetime_seconds::bigint
	 END
	 , @hashed_secret, @ip_address, @user_id, @last_used, @expires_at, @created_at, @updated_at, @login_type, @scope, @token_name) RETURNING *;

-- name: UpdateAPIKeyByID :exec
UPDATE
	api_keys
SET
	last_used = $2,
	expires_at = $3,
	ip_address = $4
WHERE
	id = $1;

-- name: DeleteAPIKeyByID :exec
DELETE FROM
	api_keys
WHERE
	id = $1;

-- name: DeleteApplicationConnectAPIKeysByUserID :exec
DELETE FROM
	api_keys
WHERE
	user_id = $1 AND
	scope = 'application_connect'::api_key_scope;

-- name: DeleteAPIKeysByUserID :exec
DELETE FROM
	api_keys
WHERE
	user_id = $1;
