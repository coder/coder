-- name: InsertAIBridgeInterception :one
INSERT INTO aibridge_interceptions (
	id, initiator_id, provider, model, metadata, started_at
) VALUES (
	@id, @initiator_id, @provider, @model, COALESCE(@metadata::jsonb, '{}'::jsonb), @started_at
)
RETURNING *;

-- name: UpdateAIBridgeInterceptionEnded :one
WITH rows AS (
    UPDATE aibridge_interceptions
	SET ended_at = NOW()
	WHERE
	    id = @id::uuid
		AND ended_at IS NULL
    RETURNING 1
)
SELECT COUNT(*) FROM rows;

-- name: InsertAIBridgeTokenUsage :one
INSERT INTO aibridge_token_usages (
  id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @input_tokens, @output_tokens, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
)
RETURNING *;

-- name: InsertAIBridgeUserPrompt :one
INSERT INTO aibridge_user_prompts (
  id, interception_id, provider_response_id, prompt, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @prompt, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
)
RETURNING *;

-- name: InsertAIBridgeToolUsage :one
INSERT INTO aibridge_tool_usages (
  id, interception_id, provider_response_id, tool, server_url, input, injected, invocation_error, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @tool, @server_url, @input, @injected, @invocation_error, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
)
RETURNING *;

-- name: GetAIBridgeInterceptionByID :one
SELECT
	*
FROM
	aibridge_interceptions
WHERE
	id = @id::uuid;

-- name: GetAIBridgeInterceptions :many
SELECT
	*
FROM
	aibridge_interceptions;

-- name: GetAIBridgeTokenUsagesByInterceptionID :many
SELECT
	*
FROM
	aibridge_token_usages WHERE interception_id = @interception_id::uuid
ORDER BY
	created_at ASC,
	id ASC;

-- name: GetAIBridgeUserPromptsByInterceptionID :many
SELECT
	*
FROM
	aibridge_user_prompts
WHERE
	interception_id = @interception_id::uuid
ORDER BY
	created_at ASC,
	id ASC;

-- name: GetAIBridgeToolUsagesByInterceptionID :many
SELECT
	*
FROM
	aibridge_tool_usages
WHERE
	interception_id = @interception_id::uuid
ORDER BY
	created_at ASC,
	id ASC;

-- name: CountAIBridgeInterceptions :one
SELECT
	COUNT(*)
FROM
	aibridge_interceptions
WHERE
	-- Filter by time frame
	CASE
		WHEN @started_after::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN aibridge_interceptions.started_at >= @started_after::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @started_before::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN aibridge_interceptions.started_at <= @started_before::timestamptz
		ELSE true
	END
	-- Filter initiator_id
	AND CASE
		WHEN @initiator_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN aibridge_interceptions.initiator_id = @initiator_id::uuid
		ELSE true
	END
	-- Filter provider
	AND CASE
		WHEN @provider::text != '' THEN aibridge_interceptions.provider = @provider::text
		ELSE true
	END
	-- Filter model
	AND CASE
		WHEN @model::text != '' THEN aibridge_interceptions.model = @model::text
		ELSE true
	END
	-- Authorize Filter clause will be injected below in ListAuthorizedAIBridgeInterceptions
	-- @authorize_filter
;

-- name: ListAIBridgeInterceptions :many
SELECT
	sqlc.embed(aibridge_interceptions),
	sqlc.embed(visible_users)
FROM
	aibridge_interceptions
JOIN
	visible_users ON visible_users.id = aibridge_interceptions.initiator_id
WHERE
	-- Filter by time frame
	CASE
		WHEN @started_after::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN aibridge_interceptions.started_at >= @started_after::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @started_before::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN aibridge_interceptions.started_at <= @started_before::timestamptz
		ELSE true
	END
	-- Filter initiator_id
	AND CASE
		WHEN @initiator_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN aibridge_interceptions.initiator_id = @initiator_id::uuid
		ELSE true
	END
	-- Filter provider
	AND CASE
		WHEN @provider::text != '' THEN aibridge_interceptions.provider = @provider::text
		ELSE true
	END
	-- Filter model
	AND CASE
		WHEN @model::text != '' THEN aibridge_interceptions.model = @model::text
		ELSE true
	END
	-- Cursor pagination
	AND CASE
		WHEN @after_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the started_at field, so select all
			-- rows before the cursor and before the after_id UUID.
			-- This uses a less than operator because we're sorting DESC. The
			-- "after_id" terminology comes from our pagination parser in
			-- coderd.
			(aibridge_interceptions.started_at, aibridge_interceptions.id) < (
				(SELECT started_at FROM aibridge_interceptions WHERE id = @after_id),
				@after_id::uuid
			)
		)
		ELSE true
	END
	-- Authorize Filter clause will be injected below in ListAuthorizedAIBridgeInterceptions
	-- @authorize_filter
ORDER BY
	aibridge_interceptions.started_at DESC,
	aibridge_interceptions.id DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100)
OFFSET @offset_
;

-- name: ListAIBridgeTokenUsagesByInterceptionIDs :many
SELECT
	*
FROM
	aibridge_token_usages
WHERE
	interception_id = ANY(@interception_ids::uuid[])
ORDER BY
	created_at ASC,
	id ASC;

-- name: ListAIBridgeUserPromptsByInterceptionIDs :many
SELECT
	*
FROM
	aibridge_user_prompts
WHERE
	interception_id = ANY(@interception_ids::uuid[])
ORDER BY
	created_at ASC,
	id ASC;

-- name: ListAIBridgeToolUsagesByInterceptionIDs :many
SELECT
	*
FROM
	aibridge_tool_usages
WHERE
	interception_id = ANY(@interception_ids::uuid[])
ORDER BY
	created_at ASC,
	id ASC;
