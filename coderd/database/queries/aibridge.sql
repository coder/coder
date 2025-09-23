-- name: InsertAIBridgeInterception :one
INSERT INTO aibridge_interceptions (id, initiator_id, provider, model, metadata, started_at)
VALUES (@id::uuid, @initiator_id::uuid, @provider, @model, COALESCE(@metadata::jsonb, '{}'::jsonb), @started_at)
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
SELECT * FROM aibridge_interceptions WHERE id = @id::uuid;

-- name: GetAIBridgeInterceptions :many
SELECT * FROM aibridge_interceptions;

-- name: GetAIBridgeTokenUsagesByInterceptionID :many
SELECT * FROM aibridge_token_usages WHERE interception_id = @interception_id::uuid;

-- name: GetAIBridgeUserPromptsByInterceptionID :many
SELECT * FROM aibridge_user_prompts WHERE interception_id = @interception_id::uuid;

-- name: GetAIBridgeToolUsagesByInterceptionID :many
SELECT * FROM aibridge_tool_usages WHERE interception_id = @interception_id::uuid;

-- name: ListAIBridgeInterceptions :many
SELECT
	aibridge_interceptions.id,
	aibridge_interceptions.initiator_id,
	aibridge_interceptions.provider,
	aibridge_interceptions.model,
	aibridge_interceptions.started_at,
	aibridge_user_prompts.prompt,
	COALESCE(aibridge_token_usages.input_tokens, 0) AS input_tokens,
	COALESCE(aibridge_token_usages.output_tokens, 0) AS output_tokens,
	aibridge_tool_usages.server_url,
	aibridge_tool_usages.tool,
	aibridge_tool_usages.input
FROM (
	SELECT
		aibridge_interceptions.id,
		aibridge_interceptions.initiator_id,
		aibridge_interceptions.provider,
		aibridge_interceptions.model,
		aibridge_interceptions.started_at
	FROM aibridge_interceptions
	WHERE
    -- Filter by time frame
	CASE
		WHEN @period_start::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN aibridge_interceptions.started_at >= @period_start::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @period_end::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN aibridge_interceptions.started_at < @period_end::timestamptz
		ELSE true
	END
	-- Filter cursor (time, uuid)
	AND CASE
		WHEN @cursor_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz AND @cursor_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid
		    THEN (aibridge_interceptions.started_at = @cursor_time::timestamptz AND aibridge_interceptions.id < @cursor_id::uuid)
			     OR aibridge_interceptions.started_at < @cursor_time::timestamptz
		ELSE true
	END
	-- Filter initiator_id
	AND CASE
		WHEN @initiator_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN aibridge_interceptions.initiator_id = @initiator_id::uuid
		ELSE true
	END
    ORDER BY aibridge_interceptions.started_at DESC, aibridge_interceptions.id DESC
    LIMIT COALESCE(NULLIF(@limit_opt::int, 0), 100)
) AS aibridge_interceptions

LEFT JOIN (
	SELECT
		aibridge_token_usages.interception_id,
	    SUM(aibridge_token_usages.input_tokens) AS input_tokens,
		SUM(aibridge_token_usages.output_tokens) AS output_tokens
	FROM aibridge_token_usages
	GROUP BY aibridge_token_usages.interception_id
) AS aibridge_token_usages ON aibridge_interceptions.id = aibridge_token_usages.interception_id

LEFT JOIN aibridge_tool_usages ON aibridge_interceptions.id = aibridge_tool_usages.interception_id
LEFT JOIN aibridge_user_prompts ON aibridge_interceptions.id = aibridge_user_prompts.interception_id

ORDER BY aibridge_interceptions.started_at DESC, aibridge_interceptions.id DESC, aibridge_tool_usages.created_at DESC;
