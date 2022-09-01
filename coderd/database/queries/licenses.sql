-- name: InsertLicense :one
INSERT INTO
	licenses (
	uploaded_at,
	jwt,
	exp
)
VALUES
	($1, $2, $3) RETURNING *;

-- name: GetLicenses :many
SELECT *
FROM licenses
ORDER BY (id);

-- name: GetUnexpiredLicenses :many
SELECT *
FROM licenses
WHERE exp > NOW()
ORDER BY (id);

-- name: DeleteLicense :one
DELETE
FROM licenses
WHERE id = $1
RETURNING id;
