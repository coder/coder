-- name: GetOAuth2ProviderApps :many
SELECT * FROM oauth2_provider_apps ORDER BY (name, id) ASC;

-- name: GetOAuth2ProviderAppByID :one
SELECT * FROM oauth2_provider_apps WHERE id = $1;

-- name: InsertOAuth2ProviderApp :one
INSERT INTO oauth2_provider_apps (
    id,
    created_at,
    updated_at,
    name,
    icon,
    callback_url
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
) RETURNING *;

-- name: UpdateOAuth2ProviderAppByID :one
UPDATE oauth2_provider_apps SET
    updated_at = $2,
    name = $3,
    icon = $4,
    callback_url = $5
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2ProviderAppByID :exec
DELETE FROM oauth2_provider_apps WHERE id = $1;

-- name: GetOAuth2ProviderAppSecretByID :one
SELECT * FROM oauth2_provider_app_secrets WHERE id = $1;

-- name: GetOAuth2ProviderAppSecretsByAppID :many
SELECT * FROM oauth2_provider_app_secrets WHERE app_id = $1 ORDER BY (created_at, id) ASC;

-- name: InsertOAuth2ProviderAppSecret :one
INSERT INTO oauth2_provider_app_secrets (
    id,
    created_at,
    hashed_secret,
    display_secret,
    app_id
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5
) RETURNING *;

-- name: UpdateOAuth2ProviderAppSecretByID :one
UPDATE oauth2_provider_app_secrets SET
    last_used_at = $2
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2ProviderAppSecretByID :exec
DELETE FROM oauth2_provider_app_secrets WHERE id = $1;
