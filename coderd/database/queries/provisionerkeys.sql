-- name: InsertProvisionerKey :one
INSERT INTO
	provisioner_keys (
		id,
        created_at,
        organization_id,
		name,
		hashed_secret,
        tags
	)
VALUES
	($1, $2, $3, lower(@name), $4, $5) RETURNING *;

-- name: GetProvisionerKeyByID :one
SELECT
    *
FROM
    provisioner_keys
WHERE
    id = $1;

-- name: GetProvisionerKeyByName :one
SELECT
    *
FROM
    provisioner_keys
WHERE
    organization_id = $1
AND 
    lower(name) = lower(@name);

-- name: ListProvisionerKeysByOrganization :many
SELECT
    *
FROM
    provisioner_keys
WHERE
    organization_id = $1;

-- name: DeleteProvisionerKey :exec
DELETE FROM
    provisioner_keys
WHERE
    id = $1;
