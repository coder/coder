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

-- name: GetOAuth2ProviderAppSecretByPrefix :one
SELECT * FROM oauth2_provider_app_secrets WHERE secret_prefix = $1;

-- name: InsertOAuth2ProviderAppSecret :one
INSERT INTO oauth2_provider_app_secrets (
    id,
    created_at,
    secret_prefix,
    hashed_secret,
    display_secret,
    app_id
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
) RETURNING *;

-- name: UpdateOAuth2ProviderAppSecretByID :one
UPDATE oauth2_provider_app_secrets SET
    last_used_at = $2
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2ProviderAppSecretByID :exec
DELETE FROM oauth2_provider_app_secrets WHERE id = $1;

-- name: GetOAuth2ProviderAppCodeByID :one
SELECT * FROM oauth2_provider_app_codes WHERE id = $1;

-- name: GetOAuth2ProviderAppCodeByPrefix :one
SELECT * FROM oauth2_provider_app_codes WHERE secret_prefix = $1;

-- name: InsertOAuth2ProviderAppCode :one
INSERT INTO oauth2_provider_app_codes (
    id,
    created_at,
    expires_at,
    secret_prefix,
    hashed_secret,
    app_id,
    user_id
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
) RETURNING *;

-- name: DeleteOAuth2ProviderAppCodeByID :exec
DELETE FROM oauth2_provider_app_codes WHERE id = $1;

-- name: DeleteOAuth2ProviderAppCodesByAppAndUserID :exec
DELETE FROM oauth2_provider_app_codes WHERE app_id = $1 AND user_id = $2;

-- name: InsertOAuth2ProviderAppToken :one
INSERT INTO oauth2_provider_app_tokens (
    id,
    created_at,
    expires_at,
    hash_prefix,
    refresh_hash,
    app_secret_id,
    api_key_id
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
) RETURNING *;

-- name: GetOAuth2ProviderAppTokenByPrefix :one
SELECT * FROM oauth2_provider_app_tokens WHERE hash_prefix = $1;

-- name: GetOAuth2ProviderAppsByUserID :many
SELECT
  COUNT(DISTINCT oauth2_provider_app_tokens.id) as token_count,
  sqlc.embed(oauth2_provider_apps)
FROM oauth2_provider_app_tokens
  INNER JOIN oauth2_provider_app_secrets
    ON oauth2_provider_app_secrets.id = oauth2_provider_app_tokens.app_secret_id
  INNER JOIN oauth2_provider_apps
    ON oauth2_provider_apps.id = oauth2_provider_app_secrets.app_id
  INNER JOIN api_keys
    ON api_keys.id = oauth2_provider_app_tokens.api_key_id
WHERE
  api_keys.user_id = $1
GROUP BY
  oauth2_provider_apps.id;

-- name: DeleteOAuth2ProviderAppTokensByAppAndUserID :exec
DELETE FROM
  oauth2_provider_app_tokens
USING
  oauth2_provider_app_secrets, api_keys
WHERE
  oauth2_provider_app_secrets.id = oauth2_provider_app_tokens.app_secret_id
  AND api_keys.id = oauth2_provider_app_tokens.api_key_id
  AND oauth2_provider_app_secrets.app_id = $1
	AND api_keys.user_id = $2;
