-- name: GetChatProjectInstructionsByProjectID :one
SELECT
    sqlc.embed(chat_project_instructions),
    visible_users.username AS updated_by_username,
    visible_users.name AS updated_by_name,
    visible_users.avatar_url AS updated_by_avatar_url
FROM chat_project_instructions
LEFT JOIN visible_users ON visible_users.id = chat_project_instructions.updated_by
WHERE chat_project_instructions.project_id = @project_id::uuid;

-- name: UpsertChatProjectInstructions :one
INSERT INTO chat_project_instructions (
    project_id,
    instructions,
    updated_by
)
VALUES (
    @project_id::uuid,
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
