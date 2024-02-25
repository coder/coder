-- name: GetUserLinkByLinkedID :one
SELECT
	user_links.*
FROM
	user_links
INNER JOIN
	users ON user_links.user_id = users.id
WHERE
	linked_id = $1
	AND
	deleted = false;

-- name: GetUserLinkByUserIDLoginType :one
SELECT
	*
FROM
	user_links
WHERE
	user_id = $1 AND login_type = $2;

-- name: GetUserLinksByUserID :many
SELECT * FROM user_links WHERE user_id = $1;

-- name: InsertUserLink :one
INSERT INTO
	user_links (
		user_id,
		login_type,
		linked_id,
		oauth_access_token,
		oauth_access_token_key_id,
		oauth_refresh_token,
		oauth_refresh_token_key_id,
		oauth_expiry,
	    debug_context
	)
VALUES
	( $1, $2, $3, $4, $5, $6, $7, $8, $9 ) RETURNING *;

-- name: UpdateUserLinkedID :one
UPDATE
	user_links
SET
	linked_id = $1
WHERE
	user_id = $2 AND login_type = $3 RETURNING *;

-- name: UpdateUserLink :one
UPDATE
	user_links
SET
	oauth_access_token = $1,
	oauth_access_token_key_id = $2,
	oauth_refresh_token = $3,
	oauth_refresh_token_key_id = $4,
	oauth_expiry = $5,
	debug_context = $6
WHERE
	user_id = $7 AND login_type = $8 RETURNING *;
