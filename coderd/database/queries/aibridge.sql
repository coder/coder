-- name: InsertAIBridgeSession :one
INSERT INTO aibridge_sessions (id, initiator_id, provider, model, started_at)
VALUES (@id::uuid, @initiator_id::uuid, @provider, @model, @started_at)
RETURNING *;

-- name: InsertAIBridgeTokenUsage :exec
INSERT INTO aibridge_token_usages (
  id, session_id, provider_id, input_tokens, output_tokens, metadata, created_at
) VALUES (
  @id, @session_id, @provider_id, @input_tokens, @output_tokens, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
);

-- name: InsertAIBridgeUserPrompt :exec
INSERT INTO aibridge_user_prompts (
  id, session_id, provider_id, prompt, metadata, created_at
) VALUES (
  @id, @session_id, @provider_id, @prompt, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
);

-- name: InsertAIBridgeToolUsage :exec
INSERT INTO aibridge_tool_usages (
  id, session_id, provider_id, tool, input, injected, metadata, created_at
) VALUES (
  @id, @session_id, @provider_id, @tool, @input, @injected, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
);

-- name: GetAIBridgeSessionByID :one
SELECT * FROM aibridge_sessions WHERE id = @id::uuid
LIMIT 1;
