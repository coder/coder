-- name: GetFileByHash :one
SELECT
	*
FROM
	files
WHERE
	hash = $1
LIMIT
	1;

-- name: InsertFile :one
INSERT INTO
	files (hash, created_at, created_by, mimetype, "data")
VALUES
	($1, $2, $3, $4, $5) RETURNING *;
