-- name: InsertTelemetryHeartbeat :exec
-- Inserts a new heartbeat event into the telemetry_heartbeats table. Replicas
-- should call this function prior to attempting to generate or publish the
-- event to the telemetry service.
-- If the query returns a duplicate primary key error, the replica should not
-- attempt to generate or publish the event to the telemetry service.
INSERT INTO
    telemetry_heartbeats (event_type, heartbeat_timestamp)
VALUES
    ($1, $2);

-- name: DeleteOldTelemetryHeartbeats :exec
-- Deletes old telemetry heartbeats from the telemetry_heartbeats table.
DELETE FROM
    telemetry_heartbeats
WHERE
    heartbeat_timestamp < @before_time::timestamptz;

-- name: ListAIBridgeInterceptionsTelemetrySummaries :many
-- Finds all unique AIBridge interception telemetry snapshots combinations
-- (provider, model, client) in the given timeframe.
SELECT
    DISTINCT ON (provider, model, client)
    provider,
    model,
    -- TODO: use the client value once we have it (see https://github.com/coder/aibridge/issues/31)
    'unknown' AS client
FROM
    aibridge_interceptions
WHERE
    started_at >= @started_at_after::timestamptz
    -- TODO: use the end time once we have it
    AND started_at < @ended_at_before::timestamptz;

-- name: CalculateAIBridgeInterceptionsTelemetrySummary :one
-- Calculates the telemetry snapshot for a given provider, model, and client
-- combination.
WITH interceptions_in_range AS (
    -- Get all matching interceptions in the given timeframe.
    SELECT
        id,
        initiator_id,
        -- TODO: use the duration value once we have it
        INTERVAL '0 seconds' AS duration
    FROM
        aibridge_interceptions
    WHERE
        provider = @provider::text
        AND model = @model::text
        -- TODO: use the client value once we have it (see https://github.com/coder/aibridge/issues/31)
        AND 'unknown' = @client::text
        AND started_at >= @started_at_after::timestamptz
        -- TODO: use the end time once we have it
        AND started_at < @ended_at_before::timestamptz
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
