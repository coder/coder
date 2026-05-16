-- name: GetUserAIProviderKeyByProviderID :one
SELECT
    *
FROM
    user_ai_provider_keys
WHERE
    user_id = @user_id::uuid
    AND ai_provider_id = @ai_provider_id::uuid;

-- name: GetUserAIProviderKeysByUserID :many
SELECT
    *
FROM
    user_ai_provider_keys
WHERE
    user_id = @user_id::uuid
ORDER BY
    ai_provider_id ASC,
    created_at ASC,
    id ASC;

-- name: GetUserAIProviderKeys :many
SELECT
    *
FROM
    user_ai_provider_keys
ORDER BY
    user_id ASC,
    ai_provider_id ASC,
    created_at ASC,
    id ASC;

-- name: UpsertUserAIProviderKey :one
INSERT INTO user_ai_provider_keys (
    id,
    user_id,
    ai_provider_id,
    api_key,
    api_key_key_id,
    created_at,
    updated_at
) VALUES (
    @id::uuid,
    @user_id::uuid,
    @ai_provider_id::uuid,
    @api_key::text,
    sqlc.narg('api_key_key_id')::text,
    @created_at::timestamptz,
    @updated_at::timestamptz
)
ON CONFLICT (user_id, ai_provider_id) DO UPDATE
SET
    api_key = EXCLUDED.api_key,
    api_key_key_id = EXCLUDED.api_key_key_id,
    updated_at = EXCLUDED.updated_at
RETURNING
    *;

-- name: UpdateUserAIProviderKey :one
UPDATE
    user_ai_provider_keys
SET
    api_key = @api_key::text,
    api_key_key_id = sqlc.narg('api_key_key_id')::text,
    updated_at = @updated_at::timestamptz
WHERE
    user_id = @user_id::uuid
    AND ai_provider_id = @ai_provider_id::uuid
RETURNING
    *;

-- name: DeleteUserAIProviderKey :exec
DELETE FROM
    user_ai_provider_keys
WHERE
    user_id = @user_id::uuid
    AND ai_provider_id = @ai_provider_id::uuid;

-- name: UpdateEncryptedUserAIProviderKey :one
UPDATE
    user_ai_provider_keys
SET
    api_key = @api_key::text,
    api_key_key_id = sqlc.narg('api_key_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;
