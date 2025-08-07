-- name: GetUserSecretByUserIDAndName :one
SELECT * FROM user_secrets
WHERE user_id = $1 AND name = $2;

-- name: GetUserSecret :one
SELECT * FROM user_secrets
WHERE id = $1;

-- name: ListUserSecrets :many
SELECT * FROM user_secrets
WHERE user_id = $1
ORDER BY name ASC;

-- name: CreateUserSecret :one
INSERT INTO user_secrets (
    id,
    user_id,
    name,
    description,
    value,
    env_name,
    file_path
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: UpdateUserSecret :one
UPDATE user_secrets
SET
    description = $2,
    value = $3,
    env_name = $4,
    file_path = $5,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteUserSecret :exec
DELETE FROM user_secrets
WHERE id = $1;
