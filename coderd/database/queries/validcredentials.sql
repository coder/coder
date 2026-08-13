-- name: InsertValidCredential :one
INSERT INTO
	valid_credentials (actor_type, actor, password)
VALUES
	($1, $2, $3) RETURNING *;

-- name: GetValidCredentialsByActor :many
-- Many, because an actor may hold more than one valid credential while a
-- rotation overlaps.
SELECT
	*
FROM
	valid_credentials
WHERE
	actor_type = $1
	AND actor = $2;
