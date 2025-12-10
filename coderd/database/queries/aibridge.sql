-- name: InsertAIBridgeInterception :one
INSERT INTO aibridge_interceptions (
	id, api_key_id, initiator_id, provider, model, metadata, started_at
) VALUES (
	@id, @api_key_id, @initiator_id, @provider, @model, COALESCE(@metadata::jsonb, '{}'::jsonb), @started_at
)
RETURNING *;

-- name: UpdateAIBridgeInterceptionEnded :one
UPDATE aibridge_interceptions
	SET ended_at = @ended_at::timestamptz
WHERE
	id = @id::uuid
	AND ended_at IS NULL
RETURNING *;

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
	-- Remove inflight interceptions (ones which lack an ended_at value).
	aibridge_interceptions.ended_at IS NOT NULL
	-- Filter by time frame
	AND CASE
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
	-- Remove inflight interceptions (ones which lack an ended_at value).
	aibridge_interceptions.ended_at IS NOT NULL
	-- Filter by time frame
	AND CASE
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

-- name: ListAIBridgeDistinctModels :many
-- Returns distinct models from aibridge_interceptions, optionally filtered by provider.
SELECT DISTINCT
	model
FROM
	aibridge_interceptions
WHERE
	-- Only include completed interceptions
	ended_at IS NOT NULL
	-- Filter by provider if specified
	AND CASE
		WHEN @provider::text != '' THEN provider = @provider::text
		ELSE true
	END
ORDER BY
	model ASC;

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

-- name: ListAIBridgeInterceptionsTelemetrySummaries :many
-- Finds all unique AI Bridge interception telemetry summaries combinations
-- (provider, model, client) in the given timeframe for telemetry reporting.
SELECT
    DISTINCT ON (provider, model, client)
    provider,
    model,
    -- TODO: use the client value once we have it (see https://github.com/coder/aibridge/issues/31)
    'unknown' AS client
FROM
    aibridge_interceptions
WHERE
    ended_at IS NOT NULL -- incomplete interceptions are not included in summaries
    AND ended_at >= @ended_at_after::timestamptz
    AND ended_at < @ended_at_before::timestamptz;

-- name: CalculateAIBridgeInterceptionsTelemetrySummary :one
-- Calculates the telemetry summary for a given provider, model, and client
-- combination for telemetry reporting.
WITH interceptions_in_range AS (
    -- Get all matching interceptions in the given timeframe.
    SELECT
        id,
        initiator_id,
        (ended_at - started_at) AS duration
    FROM
        aibridge_interceptions
    WHERE
        provider = @provider::text
        AND model = @model::text
        -- TODO: use the client value once we have it (see https://github.com/coder/aibridge/issues/31)
        AND 'unknown' = @client::text
        AND ended_at IS NOT NULL -- incomplete interceptions are not included in summaries
        AND ended_at >= @ended_at_after::timestamptz
        AND ended_at < @ended_at_before::timestamptz
),
interception_counts AS (
    SELECT
        COUNT(id) AS interception_count,
        COUNT(DISTINCT initiator_id) AS unique_initiator_count
    FROM
        interceptions_in_range
),
duration_percentiles AS (
    SELECT
        (COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM duration)), 0) * 1000)::bigint AS interception_duration_p50_millis,
        (COALESCE(PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM duration)), 0) * 1000)::bigint AS interception_duration_p90_millis,
        (COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM duration)), 0) * 1000)::bigint AS interception_duration_p95_millis,
        (COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM duration)), 0) * 1000)::bigint AS interception_duration_p99_millis
    FROM
        interceptions_in_range
),
token_aggregates AS (
    SELECT
        COALESCE(SUM(tu.input_tokens), 0) AS token_count_input,
        COALESCE(SUM(tu.output_tokens), 0) AS token_count_output,
        -- Cached tokens are stored in metadata JSON, extract if available.
        -- Read tokens may be stored in:
        -- - cache_read_input (Anthropic)
        -- - prompt_cached (OpenAI)
        COALESCE(SUM(
            COALESCE((tu.metadata->>'cache_read_input')::bigint, 0) +
            COALESCE((tu.metadata->>'prompt_cached')::bigint, 0)
        ), 0) AS token_count_cached_read,
        -- Written tokens may be stored in:
        -- - cache_creation_input (Anthropic)
        -- Note that cache_ephemeral_5m_input and cache_ephemeral_1h_input on
        -- Anthropic are included in the cache_creation_input field.
        COALESCE(SUM(
            COALESCE((tu.metadata->>'cache_creation_input')::bigint, 0)
        ), 0) AS token_count_cached_written,
        COUNT(tu.id) AS token_usages_count
    FROM
        interceptions_in_range i
    LEFT JOIN
        aibridge_token_usages tu ON i.id = tu.interception_id
),
prompt_aggregates AS (
    SELECT
        COUNT(up.id) AS user_prompts_count
    FROM
        interceptions_in_range i
    LEFT JOIN
        aibridge_user_prompts up ON i.id = up.interception_id
),
tool_aggregates AS (
    SELECT
        COUNT(tu.id) FILTER (WHERE tu.injected = true) AS tool_calls_count_injected,
        COUNT(tu.id) FILTER (WHERE tu.injected = false) AS tool_calls_count_non_injected,
        COUNT(tu.id) FILTER (WHERE tu.injected = true AND tu.invocation_error IS NOT NULL) AS injected_tool_call_error_count
    FROM
        interceptions_in_range i
    LEFT JOIN
        aibridge_tool_usages tu ON i.id = tu.interception_id
)
SELECT
    ic.interception_count::bigint AS interception_count,
    dp.interception_duration_p50_millis::bigint AS interception_duration_p50_millis,
    dp.interception_duration_p90_millis::bigint AS interception_duration_p90_millis,
    dp.interception_duration_p95_millis::bigint AS interception_duration_p95_millis,
    dp.interception_duration_p99_millis::bigint AS interception_duration_p99_millis,
    ic.unique_initiator_count::bigint AS unique_initiator_count,
    pa.user_prompts_count::bigint AS user_prompts_count,
    tok_agg.token_usages_count::bigint AS token_usages_count,
    tok_agg.token_count_input::bigint AS token_count_input,
    tok_agg.token_count_output::bigint AS token_count_output,
    tok_agg.token_count_cached_read::bigint AS token_count_cached_read,
    tok_agg.token_count_cached_written::bigint AS token_count_cached_written,
    tool_agg.tool_calls_count_injected::bigint AS tool_calls_count_injected,
    tool_agg.tool_calls_count_non_injected::bigint AS tool_calls_count_non_injected,
    tool_agg.injected_tool_call_error_count::bigint AS injected_tool_call_error_count
FROM
    interception_counts ic,
    duration_percentiles dp,
    token_aggregates tok_agg,
    prompt_aggregates pa,
    tool_aggregates tool_agg
;

-- name: DeleteOldAIBridgeRecords :one
WITH
  -- We don't have FK relationships between the dependent tables and aibridge_interceptions, so we can't rely on DELETE CASCADE.
  to_delete AS (
    SELECT id FROM aibridge_interceptions
    WHERE started_at < @before_time::timestamp with time zone
  ),
  -- CTEs are executed in order.
  tool_usages AS (
    DELETE FROM aibridge_tool_usages
    WHERE interception_id IN (SELECT id FROM to_delete)
    RETURNING 1
  ),
  token_usages AS (
    DELETE FROM aibridge_token_usages
    WHERE interception_id IN (SELECT id FROM to_delete)
    RETURNING 1
  ),
  user_prompts AS (
    DELETE FROM aibridge_user_prompts
    WHERE interception_id IN (SELECT id FROM to_delete)
    RETURNING 1
  ),
  interceptions AS (
    DELETE FROM aibridge_interceptions
    WHERE id IN (SELECT id FROM to_delete)
    RETURNING 1
  )
-- Cumulative count.
SELECT (
  (SELECT COUNT(*) FROM tool_usages) +
  (SELECT COUNT(*) FROM token_usages) +
  (SELECT COUNT(*) FROM user_prompts) +
  (SELECT COUNT(*) FROM interceptions)
)::bigint as total_deleted;
