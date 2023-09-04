/*
	This file contains a subset of users.sql queries used by the
	dbresetpw package.
*/

-- name: GetUserByEmailOrUsername :one
SELECT
	*
FROM
	users
WHERE
	(LOWER(username) = LOWER(@username) OR LOWER(email) = LOWER(@email)) AND
	deleted = false
LIMIT
	1;

-- name: UpdateUserHashedPassword :exec
UPDATE
	users
SET
	hashed_password = $2
WHERE
	id = $1;
