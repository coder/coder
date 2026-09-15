-- name: InsertChatUserMemory :one
INSERT INTO chat_user_memories (
    id,
    organization_id,
    user_id,
    name,
    description,
    body,
    source_chat_id
)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @user_id::uuid,
    @name::text,
    @description::text,
    @body::text,
    sqlc.narg('source_chat_id')::uuid
)
RETURNING *;

-- name: UpsertChatUserMemoryByName :one
INSERT INTO chat_user_memories (
    organization_id,
    user_id,
    name,
    description,
    body,
    source_chat_id
)
VALUES (
    @organization_id::uuid,
    @user_id::uuid,
    @name::text,
    @description::text,
    @body::text,
    sqlc.narg('source_chat_id')::uuid
)
ON CONFLICT (user_id, organization_id, lower(name)) DO UPDATE
SET
    description = EXCLUDED.description,
    body = EXCLUDED.body,
    source_chat_id = EXCLUDED.source_chat_id,
    updated_at = now()
RETURNING *;

-- name: GetChatUserMemoryByID :one
SELECT
    sqlc.embed(chat_user_memories),
    visible_users.username AS created_by_username
FROM chat_user_memories
JOIN visible_users ON visible_users.id = chat_user_memories.user_id
WHERE chat_user_memories.id = @id::uuid;

-- name: GetChatUserMemoryByName :one
SELECT
    sqlc.embed(chat_user_memories),
    visible_users.username AS created_by_username
FROM chat_user_memories
JOIN visible_users ON visible_users.id = chat_user_memories.user_id
WHERE chat_user_memories.user_id = @user_id::uuid
    AND chat_user_memories.organization_id = @organization_id::uuid
    AND lower(chat_user_memories.name) = lower(@name::text);

-- name: GetChatUserMemoriesByUserAndOrganization :many
SELECT
    sqlc.embed(chat_user_memories),
    visible_users.username AS created_by_username
FROM chat_user_memories
JOIN visible_users ON visible_users.id = chat_user_memories.user_id
WHERE chat_user_memories.user_id = @user_id::uuid
    AND chat_user_memories.organization_id = @organization_id::uuid
ORDER BY chat_user_memories.updated_at DESC;

-- name: CountChatUserMemoriesByUserAndOrganization :one
SELECT COUNT(*)::bigint
FROM chat_user_memories
WHERE user_id = @user_id::uuid
    AND organization_id = @organization_id::uuid;

-- name: UpdateChatUserMemoryByID :one
UPDATE chat_user_memories
SET
    name = @name::text,
    description = @description::text,
    body = @body::text,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatUserMemoryByID :exec
DELETE FROM chat_user_memories
WHERE id = @id::uuid;

-- name: DeleteChatUserMemoryByName :exec
DELETE FROM chat_user_memories
WHERE user_id = @user_id::uuid
    AND organization_id = @organization_id::uuid
    AND lower(name) = lower(@name::text);
