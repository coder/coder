-- name: GetFileByID :one
SELECT
	*
FROM
	files
WHERE
	id = $1
LIMIT
	1;

-- name: GetFileByHashAndCreator :one
SELECT
	*
FROM
	files
WHERE
	hash = $1
AND
	created_by = $2
LIMIT
	1;


-- name: InsertFile :one
INSERT INTO
	files (id, hash, created_at, created_by, mimetype, "data")
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;
