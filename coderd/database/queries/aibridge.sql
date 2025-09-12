-- name: InsertAIBridgeInterception :one
INSERT INTO aibridge_interceptions (id, initiator_id, provider, model, started_at)
VALUES (@id::uuid, @initiator_id::uuid, @provider, @model, @started_at)
RETURNING *;

-- name: InsertAIBridgeTokenUsage :exec
INSERT INTO aibridge_token_usages (
  id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @input_tokens, @output_tokens, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
);

-- name: InsertAIBridgeUserPrompt :exec
INSERT INTO aibridge_user_prompts (
  id, interception_id, provider_response_id, prompt, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @prompt, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
);

-- name: InsertAIBridgeToolUsage :exec
INSERT INTO aibridge_tool_usages (
  id, interception_id, provider_response_id, tool, server_url, input, injected, invocation_error, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @tool, @server_url, @input, @injected, @invocation_error, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
);

-- name: GetAIBridgeInterceptionByID :one
SELECT * FROM aibridge_interceptions WHERE id = @id::uuid
LIMIT 1;
