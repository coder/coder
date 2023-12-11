-- name: GetOAuth2Apps :many
SELECT * FROM oauth2_apps ORDER BY (name, id) ASC;

-- name: GetOAuth2AppByID :one
SELECT * FROM oauth2_apps WHERE id = $1;

-- name: InsertOAuth2App :one
INSERT INTO oauth2_apps (
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

-- name: UpdateOAuth2AppByID :one
UPDATE oauth2_apps SET
    updated_at = $2,
    name = $3,
    icon = $4,
    callback_url = $5
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2AppByID :exec
DELETE FROM oauth2_apps WHERE id = $1;

-- name: GetOAuth2AppSecretByID :one
SELECT * FROM oauth2_app_secrets WHERE id = $1;

-- name: GetOAuth2AppSecretsByAppID :many
SELECT * FROM oauth2_app_secrets WHERE app_id = $1 ORDER BY (created_at, id) ASC;

-- name: InsertOAuth2AppSecret :one
INSERT INTO oauth2_app_secrets (
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

-- name: UpdateOAuth2AppSecretByID :one
UPDATE oauth2_app_secrets SET
    last_used_at = $2
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2AppSecretByID :exec
DELETE FROM oauth2_app_secrets WHERE id = $1;
