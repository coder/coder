-- name: GetUserSecretByUserIDAndName :one
SELECT *
FROM user_secrets
WHERE user_id = @user_id AND name = @name;

-- name: ListUserSecrets :many
-- Returns metadata only (no value or value_key_id) for the
-- REST API list and get endpoints.
SELECT
    id, user_id, name, description,
    env_name, file_path,
    created_at, updated_at
FROM user_secrets
WHERE user_id = @user_id
ORDER BY name ASC;

-- name: ListUserSecretsWithValues :many
-- Returns all columns including the secret value. Used by the
-- provisioner (build-time injection) and the agent manifest
-- (runtime injection).
SELECT *
FROM user_secrets
WHERE user_id = @user_id
ORDER BY name ASC;

-- name: CreateUserSecret :one
INSERT INTO user_secrets (
    id,
    user_id,
    name,
    description,
    value,
    value_key_id,
    env_name,
    file_path
) VALUES (
    @id,
    @user_id,
    @name,
    @description,
    @value,
    @value_key_id,
    @env_name,
    @file_path
) RETURNING *;

-- name: UpdateUserSecretByUserIDAndName :one
UPDATE user_secrets
SET
    value       = CASE WHEN @update_value::bool THEN @value ELSE value END,
    value_key_id = CASE WHEN @update_value::bool THEN @value_key_id ELSE value_key_id END,
    description = CASE WHEN @update_description::bool THEN @description ELSE description END,
    env_name    = CASE WHEN @update_env_name::bool THEN @env_name ELSE env_name END,
    file_path   = CASE WHEN @update_file_path::bool THEN @file_path ELSE file_path END,
    updated_at  = CURRENT_TIMESTAMP
WHERE user_id = @user_id AND name = @name
RETURNING *;

-- name: DeleteUserSecretByUserIDAndName :execrows
DELETE FROM user_secrets
WHERE user_id = @user_id AND name = @name;
