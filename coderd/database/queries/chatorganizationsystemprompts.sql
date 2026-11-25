-- name: GetChatOrganizationSystemPrompt :one
SELECT *
FROM chat_organization_system_prompts
WHERE organization_id = @organization_id;

-- name: UpsertChatOrganizationSystemPrompt :one
INSERT INTO chat_organization_system_prompts (organization_id, system_prompt)
VALUES (@organization_id, @system_prompt)
ON CONFLICT (organization_id) DO UPDATE
SET system_prompt = EXCLUDED.system_prompt,
    updated_at = now()
RETURNING *;
