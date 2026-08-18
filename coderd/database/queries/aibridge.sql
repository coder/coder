-- name: InsertAIBridgeInterception :one
INSERT INTO aibridge_interceptions (
	id, api_key_id, initiator_id, provider, provider_name, model, metadata, annotations, started_at, client, client_session_id, thread_parent_id, thread_root_id, credential_kind, credential_hint, agent_firewall_session_id, agent_firewall_sequence_number
) VALUES (
	@id, @api_key_id, @initiator_id, @provider, @provider_name, @model, COALESCE(@metadata::jsonb, '{}'::jsonb), @annotations, @started_at, @client, sqlc.narg('client_session_id'), sqlc.narg('thread_parent_interception_id')::uuid, sqlc.narg('thread_root_interception_id')::uuid, @credential_kind, @credential_hint, sqlc.narg('agent_firewall_session_id')::uuid, sqlc.narg('agent_firewall_sequence_number')
)
RETURNING *;

-- name: UpdateAIBridgeInterceptionEnded :one
UPDATE aibridge_interceptions
	SET ended_at = @ended_at::timestamptz,
		-- BYOK records its hint at the start of the interception.
		-- Centralized uses key failover, so its hint is only known
		-- at end-of-interception.
		credential_hint = CASE
			WHEN credential_kind = 'centralized' THEN @credential_hint::text
			ELSE credential_hint
		END,
		-- Terminal upstream error, only set when the interception failed.
		-- NULL leaves the columns empty for successful interceptions.
		error_type = sqlc.narg('error_type')::aibridge_interception_error_type,
		error_message = sqlc.narg('error_message')::text
WHERE
	id = @id::uuid
	AND ended_at IS NULL
RETURNING *;

-- name: UpdateAIBridgeInterceptionAnnotations :one
-- Merges client-supplied work context into the annotations object. The keys
-- are built explicitly so no other annotation key can be written through
-- this query, and jsonb_strip_nulls drops the arguments left NULL so they
-- keep whatever value the row already holds.
UPDATE aibridge_interceptions
	SET annotations = annotations || jsonb_strip_nulls(jsonb_build_object(
		'linear_issue_id', sqlc.narg('linear_issue_id')::text,
		'repo', sqlc.narg('repo')::text,
		'branch', sqlc.narg('branch')::text
	))
WHERE
	id = @id::uuid
RETURNING *;

-- name: GetLatestAIBridgeInterceptionByInitiator :one
SELECT
	*
FROM
	aibridge_interceptions
WHERE
	initiator_id = @initiator_id::uuid
ORDER BY
	started_at DESC
LIMIT 1;

-- name: GetAIBridgeInterceptionLineageByToolCallID :one
-- Look up the parent interception and the root of the thread by finding
-- which interception recorded a tool usage with the given tool call ID.
-- COALESCE ensures that if the parent has no thread_root_id (i.e. it IS
-- the root), we return its own ID as the root.
SELECT aibridge_interceptions.id AS thread_parent_id,
       COALESCE(aibridge_interceptions.thread_root_id, aibridge_interceptions.id) AS thread_root_id
FROM aibridge_interceptions
WHERE aibridge_interceptions.id = (
  SELECT interception_id FROM aibridge_tool_usages
  WHERE provider_tool_call_id = @tool_call_id::text
  ORDER BY created_at DESC
  LIMIT 1
);

-- name: InsertAIBridgeTokenUsage :one
INSERT INTO aibridge_token_usages (
  id, interception_id, provider_response_id, input_tokens, output_tokens, cache_read_input_tokens, cache_write_input_tokens, metadata, created_at,
  effective_group_id, input_price_micros, output_price_micros, cache_read_price_micros, cache_write_price_micros, cost_micros
) VALUES (
  @id, @interception_id, @provider_response_id, @input_tokens, @output_tokens, @cache_read_input_tokens, @cache_write_input_tokens, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at,
  @effective_group_id, @input_price_micros, @output_price_micros, @cache_read_price_micros, @cache_write_price_micros, @cost_micros
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
  id, interception_id, provider_response_id, provider_tool_call_id, provider_item_id, tool, server_url, input, injected, invocation_error, metadata, created_at
) VALUES (
  @id, @interception_id, @provider_response_id, @provider_tool_call_id, @provider_item_id, @tool, @server_url, @input, @injected, @invocation_error, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
)
RETURNING *;

-- name: InsertAIBridgeModelThought :one
INSERT INTO aibridge_model_thoughts (
  interception_id, content, metadata, created_at
) VALUES (
  @interception_id, @content, COALESCE(@metadata::jsonb, '{}'::jsonb), @created_at
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
    COALESCE(client, 'Unknown') AS client
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
        AND COALESCE(client, 'Unknown') = @client::text
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
        COALESCE(SUM(tu.cache_read_input_tokens), 0) AS token_count_cached_read,
        COALESCE(SUM(tu.cache_write_input_tokens), 0) AS token_count_cached_written,
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
  model_thoughts AS (
    DELETE FROM aibridge_model_thoughts
    WHERE interception_id IN (SELECT id FROM to_delete)
    RETURNING 1
  ),
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
  (SELECT COUNT(*) FROM model_thoughts) +
  (SELECT COUNT(*) FROM tool_usages) +
  (SELECT COUNT(*) FROM token_usages) +
  (SELECT COUNT(*) FROM user_prompts) +
  (SELECT COUNT(*) FROM interceptions)
)::bigint as total_deleted;

-- name: CountAIBridgeSessions :one
SELECT
	COUNT(DISTINCT (aibridge_interceptions.session_id, aibridge_interceptions.initiator_id))
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
	-- Filter provider_name
	AND CASE
		WHEN @provider_name::text != '' THEN aibridge_interceptions.provider_name = @provider_name::text
		ELSE true
	END
	-- Filter model
	AND CASE
		WHEN @model::text != '' THEN aibridge_interceptions.model = @model::text
		ELSE true
	END
	-- Filter client
	AND CASE
		WHEN @client::text != '' THEN COALESCE(aibridge_interceptions.client, 'Unknown') = @client::text
		ELSE true
	END
	-- Filter session_id
	AND CASE
		WHEN @session_id::text != '' THEN aibridge_interceptions.session_id = @session_id::text
		ELSE true
	END
	-- Authorize Filter clause will be injected below in CountAuthorizedAIBridgeSessions
	-- @authorize_filter
;

-- name: ListAIBridgeSessions :many
-- Returns paginated sessions with aggregated metadata, token counts, and
-- the most recent user prompt. A "session" is a logical grouping of
-- interceptions that share the same session_id (set by the client).
--
-- Pagination-first strategy: identify the page of sessions cheaply via a
-- single GROUP BY scan, then do expensive lateral joins (tokens, prompts,
-- first-interception metadata) only for the ~page-size result set.
WITH cursor_pos AS (
	-- Resolve the cursor's last_active_at once, outside the HAVING clause,
	-- so the planner cannot accidentally re-evaluate it per group. Direct
	-- LEFT JOIN is safe here since we only use MAX/MIN aggregates (no COUNT
	-- affected by fan-out from multiple prompts per interception).
	-- COALESCE falls back to MIN(ai.started_at) so the cursor value is
	-- never NULL, which would silently drop rows from the HAVING comparison.
	SELECT COALESCE(MAX(up.created_at), MIN(ai.started_at)) AS last_active_at
	FROM aibridge_interceptions ai
	LEFT JOIN aibridge_user_prompts up ON up.interception_id = ai.id
	WHERE ai.session_id = @after_session_id AND ai.ended_at IS NOT NULL
),
session_page AS (
	-- Paginate at the session level first; only cheap aggregates here.
	-- A lateral correlated subquery for prompts keeps the join one-to-one
	-- with aibridge_interceptions so COUNT(*) for thread tallies is not
	-- inflated. LIMIT 1 combined with the (interception_id, created_at DESC)
	-- index makes this an index-only lookup per interception row rather than
	-- a full-table-scan GROUP BY over all prompts.
	-- last_active_at is the latest prompt timestamp, falling back to
	-- MIN(started_at) for sessions with no prompts. The COALESCE ensures
	-- it is never NULL so the HAVING row-value cursor comparison is safe.
	SELECT
		ai.session_id,
		ai.initiator_id,
		MIN(ai.started_at) AS started_at,
		MAX(ai.ended_at) AS ended_at,
		COUNT(*) FILTER (WHERE ai.thread_root_id IS NULL) AS threads,
		COALESCE(MAX(latest_prompt.latest_prompt_at), MIN(ai.started_at))::timestamptz AS last_active_at
	FROM
		aibridge_interceptions ai
	LEFT JOIN LATERAL (
		SELECT created_at AS latest_prompt_at
		FROM aibridge_user_prompts
		WHERE interception_id = ai.id
		ORDER BY created_at DESC
		LIMIT 1
	) latest_prompt ON true
	WHERE
		-- Remove inflight interceptions (ones which lack an ended_at value).
		ai.ended_at IS NOT NULL
		-- Filter by time frame
		AND CASE
			WHEN @started_after::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN ai.started_at >= @started_after::timestamptz
			ELSE true
		END
		AND CASE
			WHEN @started_before::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN ai.started_at <= @started_before::timestamptz
			ELSE true
		END
		-- Filter initiator_id
		AND CASE
			WHEN @initiator_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN ai.initiator_id = @initiator_id::uuid
			ELSE true
		END
		-- Filter provider
		AND CASE
			WHEN @provider::text != '' THEN ai.provider = @provider::text
			ELSE true
		END
		-- Filter provider_name
		AND CASE
			WHEN @provider_name::text != '' THEN ai.provider_name = @provider_name::text
			ELSE true
		END
		-- Filter model
		AND CASE
			WHEN @model::text != '' THEN ai.model = @model::text
			ELSE true
		END
		-- Filter client
		AND CASE
			WHEN @client::text != '' THEN COALESCE(ai.client, 'Unknown') = @client::text
			ELSE true
		END
		-- Filter session_id
		AND CASE
			WHEN @session_id::text != '' THEN ai.session_id = @session_id::text
			ELSE true
		END
		-- Authorize Filter clause will be injected below in ListAuthorizedAIBridgeSessions
		-- @authorize_filter
	GROUP BY
		ai.session_id, ai.initiator_id
	HAVING
		-- Cursor pagination: uses a composite (last_active_at, session_id) cursor to
		-- support keyset pagination. The less-than comparison matches the DESC
		-- sort order so rows after the cursor come later in results. The cursor
		-- value comes from cursor_pos to guarantee single evaluation.
		CASE
			WHEN @after_session_id::text != '' THEN (
				(COALESCE(MAX(latest_prompt.latest_prompt_at), MIN(ai.started_at)), ai.session_id) < (
					(SELECT last_active_at FROM cursor_pos),
					@after_session_id::text
				)
			)
			ELSE true
		END
	ORDER BY
		last_active_at DESC,
		ai.session_id DESC
	LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100)
	OFFSET @offset_
)
SELECT
	sp.session_id,
	visible_users.id AS user_id,
	visible_users.username AS user_username,
	visible_users.name AS user_name,
	visible_users.avatar_url AS user_avatar_url,
	sr.providers::text[] AS providers,
	sr.models::text[] AS models,
	COALESCE(sr.client, '')::varchar(64) AS client,
	sr.metadata::jsonb AS metadata,
	sp.started_at::timestamptz AS started_at,
	sp.ended_at::timestamptz AS ended_at,
	sp.threads,
	COALESCE(st.input_tokens, 0)::bigint AS input_tokens,
	COALESCE(st.output_tokens, 0)::bigint AS output_tokens,
	COALESCE(st.cache_read_input_tokens, 0)::bigint AS cache_read_input_tokens,
	COALESCE(st.cache_write_input_tokens, 0)::bigint AS cache_write_input_tokens,
	COALESCE(slp.prompt, '') AS last_prompt,
	sp.last_active_at AS last_active_at,
	COALESCE(bnc.total, 0)::bigint AS network_calls_total,
	COALESCE(bnc.blocked, 0)::bigint AS network_calls_blocked,
	COALESCE(sr.firewall_active, false) AS firewall_active
FROM
	session_page sp
JOIN
	visible_users ON visible_users.id = sp.initiator_id
LEFT JOIN LATERAL (
	SELECT
		(ARRAY_AGG(ai.client ORDER BY ai.started_at, ai.id))[1] AS client,
		(ARRAY_AGG(ai.metadata ORDER BY ai.started_at, ai.id))[1] AS metadata,
		ARRAY_AGG(DISTINCT ai.provider ORDER BY ai.provider) AS providers,
		ARRAY_AGG(DISTINCT ai.model ORDER BY ai.model) AS models,
		ARRAY_AGG(ai.id) AS interception_ids,
		BOOL_OR(ai.agent_firewall_session_id IS NOT NULL) AS firewall_active
	FROM aibridge_interceptions ai
	WHERE ai.session_id = sp.session_id
		AND ai.initiator_id = sp.initiator_id
		AND ai.ended_at IS NOT NULL
) sr ON true
LEFT JOIN LATERAL (
	-- Aggregate tokens only for this session's interceptions.
	SELECT
		COALESCE(SUM(tu.input_tokens), 0)::bigint AS input_tokens,
		COALESCE(SUM(tu.output_tokens), 0)::bigint AS output_tokens,
		COALESCE(SUM(tu.cache_read_input_tokens), 0)::bigint AS cache_read_input_tokens,
		COALESCE(SUM(tu.cache_write_input_tokens), 0)::bigint AS cache_write_input_tokens
	FROM aibridge_token_usages tu
	WHERE tu.interception_id = ANY(sr.interception_ids)
) st ON true
LEFT JOIN LATERAL (
	-- Fetch only the most recent user prompt across all interceptions
	-- in the session.
	SELECT up.prompt
	FROM aibridge_user_prompts up
	WHERE up.interception_id = ANY(sr.interception_ids)
	ORDER BY up.created_at DESC, up.id DESC
	LIMIT 1
) slp ON true
LEFT JOIN LATERAL (
	-- Count Agent Firewall network calls attributed to this session. Each
	-- interception marks a point in its firewall session's monotonic sequence
	-- stream; the boundary logs it triggered fall in the open interval
	-- (this seq, next interception's seq) within the same firewall session.
	-- The exclusive lower bound drops the interception's own LLM-provider call
	-- (logged at exactly its sequence number), leaving the agent's other
	-- egress. next_seq considers all interceptions in the firewall session so
	-- windows never bleed across AI sessions that share one firewall session.
	--
-- The last interception in a session has no next row, so next_seq uses
-- the largest sequence_number instead of NULL. The lookup stays a plain
-- range, so the (session_id, sequence_number) index answers it alone.
-- With NULL and an OR check, the index cannot bound the range: each
-- interception reads every log to the end of the session and throws
-- most of them away.
	SELECT
		COUNT(*)::bigint AS total,
		COUNT(*) FILTER (WHERE bl.matched_rule IS NULL)::bigint AS blocked
	FROM aibridge_interceptions afi
	LEFT JOIN LATERAL (
		SELECT COALESCE(MIN(nxt.agent_firewall_sequence_number), 2147483647) AS next_seq
		FROM aibridge_interceptions nxt
		WHERE nxt.agent_firewall_session_id = afi.agent_firewall_session_id
			AND nxt.agent_firewall_sequence_number > afi.agent_firewall_sequence_number
	) w ON true
	JOIN boundary_logs bl
		ON bl.session_id = afi.agent_firewall_session_id
		AND bl.sequence_number > afi.agent_firewall_sequence_number
		AND bl.sequence_number < w.next_seq
	WHERE afi.id = ANY(sr.interception_ids)
		AND afi.agent_firewall_session_id IS NOT NULL
		AND afi.agent_firewall_sequence_number IS NOT NULL
) bnc ON true
ORDER BY
	sp.last_active_at DESC,
	sp.session_id DESC
;

-- name: GetAIBridgeSessionTopDomains :many
-- Returns the most contacted destination hosts for an AI session, ordered by
-- call count descending and limited to the top @limit_ rows. total_domains is
-- the number of distinct domains across the whole session, used to render a
-- "+N more" overflow beyond the returned rows. Only HTTP egress is considered;
-- dns/git/fs boundary logs do not carry a domain in the same shape.
--
-- Windowing mirrors the network_calls aggregation in ListAIBridgeSessions:
-- each interception's boundary logs fall in the open interval (this seq, next
-- interception's seq) within the same firewall session. The exclusive lower
-- bound drops the interception's own LLM-provider call. next_seq considers all
-- interceptions in the firewall session so windows never bleed across AI
-- sessions that share one firewall session, and falls back to the maximum
-- sequence_number for the last interception so the window stays an
-- index-satisfiable range.
WITH session_boundary_logs AS (
	SELECT bl.detail
	FROM aibridge_interceptions afi
	LEFT JOIN LATERAL (
		SELECT COALESCE(MIN(nxt.agent_firewall_sequence_number), 2147483647) AS next_seq
		FROM aibridge_interceptions nxt
		WHERE nxt.agent_firewall_session_id = afi.agent_firewall_session_id
			AND nxt.agent_firewall_sequence_number > afi.agent_firewall_sequence_number
	) w ON true
	JOIN boundary_logs bl
		ON bl.session_id = afi.agent_firewall_session_id
		AND bl.sequence_number > afi.agent_firewall_sequence_number
		AND bl.sequence_number < w.next_seq
	WHERE afi.session_id = @session_id::text
		AND afi.ended_at IS NOT NULL
		AND afi.agent_firewall_session_id IS NOT NULL
		AND afi.agent_firewall_sequence_number IS NOT NULL
		AND bl.proto = 'http'
),
extracted AS (
	-- Strip an optional scheme, then keep the host up to the first port, path,
	-- query, or fragment delimiter. This assumes HTTP egress detail is a plain
	-- scheme+host(+port) URL: it does not handle userinfo (user@host, which
	-- would be captured into the host) or IPv6 literal hosts ([::1], where the
	-- leading '[' is captured and the ':' terminates early). Boundary HTTP logs
	-- do not currently emit those forms; revisit this extraction if they do.
	SELECT substring(detail from '^(?:[A-Za-z][A-Za-z0-9+.-]*://)?([^/:?#]+)') AS domain
	FROM session_boundary_logs
),
domains AS (
	SELECT domain, COUNT(*)::bigint AS count
	FROM extracted
	WHERE domain IS NOT NULL AND domain != ''
	GROUP BY domain
)
SELECT
	-- COALESCE keeps sqlc from typing the grouped column as nullable; the
	-- domains CTE already filters out NULL/empty hosts.
	COALESCE(domain, '')::text AS domain,
	count,
	COUNT(*) OVER ()::bigint AS total_domains
FROM domains
ORDER BY count DESC, domain ASC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 5);

-- name: ListAIBridgeSessionNetworkCalls :many
-- Returns the individual Agent Firewall network calls made during an AI
-- session, ordered chronologically. All protocols are included, unlike
-- GetAIBridgeSessionTopDomains which considers only HTTP egress, so the list
-- covers the same events the network_calls summary in ListAIBridgeSessions
-- counts. The list is capped at @limit_ rows, so its length equals the summary
-- total only for sessions at or below the cap. The summary stays authoritative
-- for whole-session totals.
--
-- Windowing mirrors that summary and GetAIBridgeSessionTopDomains: each
-- interception's boundary logs fall in the open interval (this seq, next
-- interception's seq) within the same firewall session. The exclusive lower
-- bound drops the interception's own LLM-provider call. next_seq considers all
-- interceptions in the firewall session so windows never bleed across AI
-- sessions that share one firewall session, and falls back to the maximum
-- sequence_number for the last interception so the window stays an
-- index-satisfiable range.
SELECT bl.*
FROM aibridge_interceptions afi
LEFT JOIN LATERAL (
	SELECT COALESCE(MIN(nxt.agent_firewall_sequence_number), 2147483647) AS next_seq
	FROM aibridge_interceptions nxt
	WHERE nxt.agent_firewall_session_id = afi.agent_firewall_session_id
		AND nxt.agent_firewall_sequence_number > afi.agent_firewall_sequence_number
) w ON true
JOIN boundary_logs bl
	ON bl.session_id = afi.agent_firewall_session_id
	AND bl.sequence_number > afi.agent_firewall_sequence_number
	AND bl.sequence_number < w.next_seq
WHERE afi.session_id = @session_id::text
	AND afi.ended_at IS NOT NULL
	AND afi.agent_firewall_session_id IS NOT NULL
	AND afi.agent_firewall_sequence_number IS NOT NULL
-- created_at leads because a session can span several firewall sessions, whose
-- sequence numbers are independent streams. id breaks remaining ties so the row
-- that lands on the limit boundary is stable across identical requests.
ORDER BY bl.created_at ASC, bl.sequence_number ASC, bl.id ASC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 1000);

-- name: ListAIBridgeSessionThreads :many
-- Returns all interceptions belonging to paginated threads within a session.
-- Threads are paginated by (started_at, thread_id) cursor.
WITH paginated_threads AS (
	SELECT
		-- Find thread root interceptions (thread_root_id IS NULL), apply cursor
		-- pagination, and return the page.
		aibridge_interceptions.id AS thread_id,
		aibridge_interceptions.started_at
	FROM
		aibridge_interceptions
	WHERE
		aibridge_interceptions.session_id = @session_id::text
		AND aibridge_interceptions.ended_at IS NOT NULL
		AND aibridge_interceptions.thread_root_id IS NULL
		-- Pagination cursor.
		AND (@after_id::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR
			(aibridge_interceptions.started_at, aibridge_interceptions.id) > (
				(SELECT started_at FROM aibridge_interceptions ai2 WHERE ai2.id = @after_id),
				@after_id::uuid
			)
		)
		AND (@before_id::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR
			(aibridge_interceptions.started_at, aibridge_interceptions.id) < (
				(SELECT started_at FROM aibridge_interceptions ai2 WHERE ai2.id = @before_id),
				@before_id::uuid
			)
		)
		-- @authorize_filter
	ORDER BY
		aibridge_interceptions.started_at ASC,
		aibridge_interceptions.id ASC
	LIMIT COALESCE(NULLIF(@limit_::integer, 0), 50)
)
SELECT
	COALESCE(aibridge_interceptions.thread_root_id, aibridge_interceptions.id) AS thread_id,
	sqlc.embed(aibridge_interceptions)
FROM
	aibridge_interceptions
JOIN
	paginated_threads pt
		ON pt.thread_id = COALESCE(aibridge_interceptions.thread_root_id, aibridge_interceptions.id)
WHERE
	aibridge_interceptions.session_id = @session_id::text
	AND aibridge_interceptions.ended_at IS NOT NULL
	-- @authorize_filter
ORDER BY
	-- Ensure threads and their associated interceptions (agentic loops) are sorted chronologically.
	pt.started_at ASC,
	pt.thread_id ASC,
	aibridge_interceptions.started_at ASC,
	aibridge_interceptions.id ASC
;

-- name: ListAIBridgeModelThoughtsByInterceptionIDs :many
SELECT
	*
FROM
	aibridge_model_thoughts
WHERE
	interception_id = ANY(@interception_ids::uuid[])
ORDER BY
	created_at ASC;

-- name: ListAIBridgeModels :many
SELECT
	model
FROM
	aibridge_interceptions
WHERE
	-- Remove inflight interceptions (ones which lack an ended_at value).
	aibridge_interceptions.ended_at IS NOT NULL
	-- Filter model
	AND CASE
		WHEN @model::text != '' THEN aibridge_interceptions.model LIKE @model::text || '%'
		ELSE true
	END
	-- We use an `@authorize_filter` as we are attempting to list models that are relevant
	-- to the user and what they are allowed to see.
	-- Authorize Filter clause will be injected below in ListAIBridgeModelsAuthorized
	-- @authorize_filter
GROUP BY
	model
ORDER BY
	model ASC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100)
OFFSET @offset_
;


-- name: ListAIBridgeClients :many
SELECT
	COALESCE(client, 'Unknown') AS client
FROM
	aibridge_interceptions
WHERE
	ended_at IS NOT NULL
	-- Filter client (prefix match to allow B-tree index usage).
	AND CASE
		WHEN @client::text != '' THEN COALESCE(aibridge_interceptions.client, 'Unknown') LIKE @client::text || '%'
		ELSE true
	END
	-- We use an `@authorize_filter` as we are attempting to list clients
	-- that are relevant to the user and what they are allowed to see.
	-- Authorize Filter clause will be injected below in
	-- ListAIBridgeClientsAuthorized.
	-- @authorize_filter
GROUP BY
	client
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100)
OFFSET @offset_
;

-- name: GetAIBridgeChatCost :one
-- AI Gateway cost for one chat tree: the root chat plus every subagent
-- beneath it. The spawning chat's ID is recorded as the interception session
-- ID (see chatprovider.CoderHeaders), so a subagent's requests are attributed
-- to its parent rather than the root, and only whole trees can be summed. The
-- owner check guards against session-id collisions. Usage without an
-- effective group never reaches ai_user_daily_spend.
WITH per_request AS (
	-- One row per interception. A request records one token usage per provider
	-- response, so aggregating here keeps the outer counts per request and
	-- flags a request whose cost is partial because some usage was unpriced.
	-- The usage join is a LEFT JOIN so a request that ended without eligible
	-- usage, such as one that failed upstream, still counts as a request. The
	-- tu.id guard keeps that row from reading as unpriced usage, since the
	-- unmatched side is all NULL.
	SELECT
		SUM(tu.cost_micros) AS cost_micros,
		BOOL_OR(tu.id IS NOT NULL AND tu.cost_micros IS NULL) AS has_unpriced_usage
	FROM aibridge_interceptions i
	JOIN chats c ON c.id::text = i.session_id AND c.owner_id = i.initiator_id
	LEFT JOIN aibridge_token_usages tu ON tu.interception_id = i.id AND tu.effective_group_id IS NOT NULL
	WHERE (
			-- Spelled out instead of COALESCE(c.root_chat_id, c.id) so each branch
			-- stays a plain comparison against an indexed column.
			c.root_chat_id = @root_chat_id::uuid
			OR (c.root_chat_id IS NULL AND c.id = @root_chat_id::uuid)
		)
		-- Restrict to aibridge.ClientCoderAgents so another client's session
		-- reference cannot match a chat ID.
		AND i.client = 'Coder Agents'
		AND i.ended_at IS NOT NULL
	GROUP BY i.id
)
SELECT
	COALESCE(SUM(cost_micros), 0)::bigint AS total_cost_micros,
	COUNT(*)::bigint AS request_count,
	COUNT(*) FILTER (WHERE has_unpriced_usage)::bigint AS unpriced_request_count
FROM per_request;
