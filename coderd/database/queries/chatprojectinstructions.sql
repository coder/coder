-- name: GetChatProjectInstructionsByProjectID :one
SELECT
    sqlc.embed(chat_project_instructions),
    users.username AS updated_by_username,
    users.name AS updated_by_name,
    users.avatar_url AS updated_by_avatar_url
FROM chat_project_instructions
-- Users are soft deleted, so ON DELETE SET NULL never clears updated_by.
-- Excluding deleted users here reports their edits as anonymous.
LEFT JOIN users ON users.id = chat_project_instructions.updated_by
    AND users.deleted = false
WHERE chat_project_instructions.project_id = @project_id::uuid;

-- name: UpsertChatProjectInstructions :one
INSERT INTO chat_project_instructions (
    project_id,
    organization_id,
    instructions,
    updated_by
)
VALUES (
    @project_id::uuid,
    @organization_id::uuid,
    @instructions::text,
    @updated_by::uuid
)
ON CONFLICT (project_id) DO UPDATE SET
    instructions = EXCLUDED.instructions,
    updated_by = EXCLUDED.updated_by,
    updated_at = now()
RETURNING *;

-- name: DeleteChatProjectInstructionsByProjectID :exec
DELETE FROM chat_project_instructions
WHERE project_id = @project_id::uuid;
