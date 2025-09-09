-- name: ListExternalAuthDcrClients :many
SELECT
    *
FROM
    external_auth_dcr_clients;

-- name: InsertExternalAuthDcrClient :one
INSERT INTO external_auth_dcr_clients (
    provider_id,
    client_id,
    client_secret,
    client_secret_key_id,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteExternalAuthDcrClient :exec
DELETE FROM
    external_auth_dcr_clients
WHERE
    provider_id = $1;
