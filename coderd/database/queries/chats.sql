-- name: InsertChat :one
INSERT INTO chats (
	id,
	created_at,
	updated_at,
	organization_id,
	owner_id,
	workspace_id,
	title,
	provider,
	model,
	metadata
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetChatByID :one
SELECT * FROM chats WHERE id = $1;

-- name: ListChatsByOwner :many
SELECT * FROM chats
WHERE owner_id = $1
ORDER BY updated_at DESC, id DESC;

-- name: UpdateChatWorkspaceID :one
UPDATE chats
SET
	workspace_id = $2,
	updated_at = $3
FROM
	workspaces w
WHERE
	chats.id = $1
	AND chats.workspace_id IS NULL
	AND w.id = $2
	AND w.organization_id = chats.organization_id
	AND w.owner_id = chats.owner_id
RETURNING
	chats.*;

-- name: InsertChatMessage :one
INSERT INTO chat_messages (
	chat_id,
	created_at,
	role,
	content
) VALUES (
	$1, $2, $3, $4
)
RETURNING *;

-- name: ListChatMessages :many
SELECT * FROM chat_messages
WHERE chat_id = $1
ORDER BY id ASC;

-- name: ListChatMessagesAfter :many
SELECT * FROM chat_messages
WHERE chat_id = $1
	AND id > $2
ORDER BY id ASC;
