-- name: InsertLicense :one
INSERT INTO
	licenses (
	uploaded_at,
	jwt,
	exp,
	uuid
)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: GetLicenses :many
SELECT *
FROM licenses
ORDER BY (id);

-- name: GetLicenseByID :one
SELECT
	*
FROM
	licenses
WHERE
	id = $1
LIMIT
	1;

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
