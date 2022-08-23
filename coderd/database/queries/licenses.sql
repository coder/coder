-- name: InsertLicense :one
INSERT INTO
	licenses (
	uploaded_at,
	jwt,
	exp
)
VALUES
	($1, $2, $3) RETURNING *;
