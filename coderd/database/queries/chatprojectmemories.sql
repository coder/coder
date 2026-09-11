-- name: InsertChatProjectMemory :one
INSERT INTO chat_project_memories (
    id,
    project_id,
    organization_id,
    name,
    description,
    body,
    source_chat_id,
    created_by
)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @project_id::uuid,
    @organization_id::uuid,
    @name::text,
    @description::text,
    @body::text,
    sqlc.narg('source_chat_id')::uuid,
    @created_by::uuid
)
RETURNING *;

-- name: UpsertChatProjectMemoryByName :one
INSERT INTO chat_project_memories (
    project_id,
    organization_id,
    name,
    description,
    body,
    source_chat_id,
    created_by
)
VALUES (
    @project_id::uuid,
    @organization_id::uuid,
    @name::text,
    @description::text,
    @body::text,
    sqlc.narg('source_chat_id')::uuid,
    @created_by::uuid
)
ON CONFLICT (project_id, lower(name)) DO UPDATE
SET
    description = EXCLUDED.description,
    body = EXCLUDED.body,
    source_chat_id = EXCLUDED.source_chat_id,
    updated_at = now()
RETURNING *;

-- name: GetChatProjectMemoryByID :one
SELECT
    sqlc.embed(chat_project_memories),
    visible_users.username AS created_by_username
FROM chat_project_memories
JOIN visible_users ON visible_users.id = chat_project_memories.created_by
WHERE chat_project_memories.id = @id::uuid;

-- name: GetChatProjectMemoryByName :one
SELECT
    sqlc.embed(chat_project_memories),
    visible_users.username AS created_by_username
FROM chat_project_memories
JOIN visible_users ON visible_users.id = chat_project_memories.created_by
WHERE chat_project_memories.project_id = @project_id::uuid
    AND lower(chat_project_memories.name) = lower(@name::text);

-- name: GetChatProjectMemoriesByProjectID :many
SELECT
    sqlc.embed(chat_project_memories),
    visible_users.username AS created_by_username
FROM chat_project_memories
JOIN visible_users ON visible_users.id = chat_project_memories.created_by
WHERE chat_project_memories.project_id = @project_id::uuid
ORDER BY chat_project_memories.updated_at DESC;

-- name: CountChatProjectMemoriesByProjectID :one
SELECT COUNT(*)::bigint
FROM chat_project_memories
WHERE project_id = @project_id::uuid;

-- name: UpdateChatProjectMemoryByID :one
UPDATE chat_project_memories
SET
    name = @name::text,
    description = @description::text,
    body = @body::text,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatProjectMemoryByID :exec
DELETE FROM chat_project_memories
WHERE id = @id::uuid;

-- name: DeleteChatProjectMemoryByName :exec
DELETE FROM chat_project_memories
WHERE project_id = @project_id::uuid
    AND lower(name) = lower(@name::text);

-- name: GetChatProjectMemoryCursor :one
SELECT *
FROM chat_project_memory_cursors
WHERE chat_id = @chat_id::uuid;

-- name: UpsertChatProjectMemoryCursor :one
INSERT INTO chat_project_memory_cursors (chat_id, history_version)
VALUES (@chat_id::uuid, @history_version::bigint)
ON CONFLICT (chat_id) DO UPDATE
SET
    history_version = EXCLUDED.history_version,
    extracted_at = now()
RETURNING *;
