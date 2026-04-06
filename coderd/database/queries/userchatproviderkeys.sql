-- name: GetUserChatProviderKeys :many
SELECT * FROM user_chat_provider_keys WHERE user_id = @user_id ORDER BY created_at ASC, id ASC;

-- name: UpsertUserChatProviderKey :one
INSERT INTO user_chat_provider_keys (user_id, chat_provider_id, api_key, api_key_key_id)
VALUES (@user_id, @chat_provider_id, @api_key, sqlc.narg('api_key_key_id')::text)
ON CONFLICT (user_id, chat_provider_id) DO UPDATE SET
    api_key = @api_key,
    api_key_key_id = sqlc.narg('api_key_key_id')::text,
    updated_at = NOW()
RETURNING *;

-- name: UpdateUserChatProviderKey :one
UPDATE user_chat_provider_keys
SET api_key = @api_key, api_key_key_id = sqlc.narg('api_key_key_id')::text, updated_at = NOW()
WHERE user_id = @user_id AND chat_provider_id = @chat_provider_id
RETURNING *;

-- name: DeleteUserChatProviderKey :exec
DELETE FROM user_chat_provider_keys WHERE user_id = @user_id AND chat_provider_id = @chat_provider_id;
