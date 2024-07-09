-- name: InsertProvisionerKey :one
INSERT INTO
	provisioner_keys (
		id,
        created_at,
        organization_id,
		name,
		hashed_secret
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: GetProvisionerKeyByName :one
SELECT
    *
FROM
    provisioner_keys
WHERE
    organization_id = $1
AND 
    name = $2;

-- name: ListProvisionerKeysByOrganization :many
SELECT
    id,
    created_at,
    organization_id,
    name
FROM
    provisioner_keys
WHERE
    organization_id = $1;

-- name: DeleteProvisionerKey :exec
DELETE FROM
    provisioner_keys
WHERE
    id = $1;
