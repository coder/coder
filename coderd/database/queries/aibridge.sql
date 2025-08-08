-- name: InsertAIBridgeSession :one
INSERT INTO aibridge_sessions (id, initiator_id, provider, model)
VALUES (@id::uuid, @initiator_id::uuid, @provider, @model)
RETURNING @id::uuid;

-- name: InsertAIBridgeTokenUsage :exec
INSERT INTO aibridge_token_usages (
  id, session_id, provider_id, input_tokens, output_tokens, metadata
) VALUES (
  @id, @session_id, @provider_id, @input_tokens, @output_tokens, COALESCE(@metadata::jsonb, '{}'::jsonb)
);

-- name: InsertAIBridgeUserPrompt :exec
INSERT INTO aibridge_user_prompts (
  id, session_id, provider_id, prompt, metadata
) VALUES (
  @id, @session_id, @provider_id, @prompt, COALESCE(@metadata::jsonb, '{}'::jsonb)
);

-- name: InsertAIBridgeToolUsage :exec
INSERT INTO aibridge_tool_usages (
  id, session_id, provider_id, tool, input, injected, metadata
) VALUES (
  @id, @session_id, @provider_id, @tool, @input, @injected, COALESCE(@metadata::jsonb, '{}'::jsonb)
);
