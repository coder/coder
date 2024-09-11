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

-- name: GetProvisionerKeyByHashedSecret :one
SELECT
    *
FROM
    provisioner_keys
WHERE
    hashed_secret = $1;

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
    organization_id = $1
AND
    id != '11111111-1111-1111-1111-111111111111'::uuid
AND 
    id != '22222222-2222-2222-2222-222222222222'::uuid
AND 
    id != '33333333-3333-3333-3333-333333333333'::uuid;

-- name: DeleteProvisionerKey :exec
DELETE FROM
    provisioner_keys
WHERE
    id = $1;
