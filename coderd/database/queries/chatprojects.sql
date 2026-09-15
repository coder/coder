-- name: InsertChatProject :one
INSERT INTO chat_projects (id, organization_id, created_by, name, description)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @created_by::uuid,
    @name::text,
    @description::text
)
RETURNING *;

-- name: GetChatProjectByID :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid;

-- name: GetChatProjectsByOrganizationID :many
SELECT
    sqlc.embed(chat_projects),
    COUNT(chats.id)::bigint AS chat_count
FROM chat_projects
LEFT JOIN chats ON chats.project_id = chat_projects.id
    AND chats.parent_chat_id IS NULL
    AND chats.archived = false
WHERE chat_projects.organization_id = @organization_id::uuid
GROUP BY chat_projects.id
ORDER BY lower(chat_projects.name);

-- name: UpdateChatProjectByID :one
UPDATE chat_projects
SET
    name = @name::text,
    description = @description::text,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatProjectByID :exec
DELETE FROM chat_projects
WHERE id = @id::uuid;
