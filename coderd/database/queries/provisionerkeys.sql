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

-- name: ListProvisionerKeysByOrganizationExcludeReserved :many
SELECT
    *
FROM
    provisioner_keys
WHERE
    organization_id = $1
AND
    -- exclude reserved built-in key
    id != '00000000-0000-0000-0000-000000000001'::uuid
AND
    -- exclude reserved user-auth key
    id != '00000000-0000-0000-0000-000000000002'::uuid
AND
    -- exclude reserved psk key
    id != '00000000-0000-0000-0000-000000000003'::uuid;

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
