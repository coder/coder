-- name: InsertChat :one
INSERT INTO chats (owner_id, created_at, updated_at, title)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateChatByID :exec
UPDATE chats
SET title = $2, updated_at = $3
WHERE id = $1;

-- name: GetChatsByOwnerID :many
SELECT * FROM chats
WHERE owner_id = $1
ORDER BY created_at DESC;

-- name: GetChatByID :one
SELECT * FROM chats
WHERE id = $1;

-- name: InsertChatMessages :many
INSERT INTO chat_messages (chat_id, created_at, model, provider, content)
SELECT
    @chat_id :: uuid AS chat_id,
    @created_at :: timestamptz AS created_at,
    @model :: VARCHAR(127) AS model,
    @provider :: VARCHAR(127) AS provider,
    jsonb_array_elements(@content :: jsonb) AS content
RETURNING chat_messages.*;

-- name: GetChatMessagesByChatID :many
SELECT * FROM chat_messages
WHERE chat_id = $1
ORDER BY created_at ASC;

-- name: DeleteChat :exec
DELETE FROM chats WHERE id = $1;
