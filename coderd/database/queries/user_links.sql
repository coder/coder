-- name: GetUserLinkByLinkedID :one
SELECT
	*
FROM
	user_links
WHERE
	linked_id = $1;

-- name: GetUserLinkByUserIDLoginType :one
SELECT
	*
FROM
	user_links
WHERE
	user_id = $1 AND login_type = $2;

-- name: InsertUserLink :one
INSERT INTO
	user_links (
		user_id,
		login_type,
		linked_id,
		oauth_access_token,
		oauth_refresh_token,
		oauth_expiry
	)
VALUES
	( $1, $2, $3, $4, $5, $6 ) RETURNING *;

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
	oauth_refresh_token = $2,
	oauth_expiry = $3
WHERE
	user_id = $4 AND login_type = $5 RETURNING *;
