-- PR Insights queries for the /agents analytics dashboard.
-- These aggregate data from chat_diff_statuses (PR metadata) joined
-- with chats and chat_messages (cost) to power the PR Insights view.
--
-- Cost is computed per PR by summing the PR-linked chat's own cost plus
-- the costs of any direct children (subagents) it spawned that do NOT
-- have their own PR association. If a child chat has its own
-- chat_diff_statuses entry (with a non-NULL pull_request_state), its
-- cost is attributed to that child's PR instead — preventing
-- double-counting when sibling chats create different PRs.
-- Subagent trees are at most 2 levels deep (enforced by the
-- application layer). PR metadata (state, additions, deletions)
-- comes from the most recent chat via DISTINCT ON so that each PR
-- is counted exactly once.

-- name: GetPRInsightsSummary :one
-- Returns aggregate PR metrics for the given date range.
-- The handler calls this twice (current + previous period) for trends.
-- Uses two CTEs: pr_costs sums cost for the PR-linked chat and its
-- direct children (that lack their own PR), and deduped picks one row
-- per PR for state/additions/deletions.
WITH pr_costs AS (
    SELECT
        prc.pr_key,
        COALESCE(SUM(cc.cost_micros), 0) AS cost_micros
    FROM (
        -- For each PR, include the chat that references it plus any
        -- direct children (subagents) that do not have their own PR.
        SELECT DISTINCT
            COALESCE(NULLIF(cds.url, ''), c.id::text) AS pr_key,
            related.id AS chat_id
        FROM chat_diff_statuses cds
        JOIN chats c ON c.id = cds.chat_id
        JOIN chats related
          ON related.id = c.id
          OR (related.parent_chat_id = c.id
              AND NOT EXISTS (
                  SELECT 1 FROM chat_diff_statuses cds2
                  WHERE cds2.chat_id = related.id
                    AND cds2.pull_request_state IS NOT NULL
              ))
        WHERE cds.pull_request_state IS NOT NULL
          AND c.created_at >= @start_date::timestamptz
          AND c.created_at < @end_date::timestamptz
          AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ) prc
    LEFT JOIN LATERAL (
        SELECT COALESCE(SUM(cm.total_cost_micros), 0) AS cost_micros
        FROM chat_messages cm
        WHERE cm.chat_id = prc.chat_id
          AND cm.total_cost_micros IS NOT NULL
    ) cc ON TRUE
    GROUP BY prc.pr_key
),
deduped AS (
    SELECT DISTINCT ON (COALESCE(NULLIF(cds.url, ''), c.id::text))
        COALESCE(NULLIF(cds.url, ''), c.id::text) AS pr_key,
        cds.pull_request_state,
        cds.additions,
        cds.deletions
    FROM chat_diff_statuses cds
    JOIN chats c ON c.id = cds.chat_id
    WHERE cds.pull_request_state IS NOT NULL
      AND c.created_at >= @start_date::timestamptz
      AND c.created_at < @end_date::timestamptz
      AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ORDER BY COALESCE(NULLIF(cds.url, ''), c.id::text), c.created_at DESC, c.id DESC
)
SELECT
    COUNT(*)::bigint AS total_prs_created,
    COUNT(*) FILTER (WHERE d.pull_request_state = 'merged')::bigint AS total_prs_merged,
    COUNT(*) FILTER (WHERE d.pull_request_state = 'closed')::bigint AS total_prs_closed,
    COALESCE(SUM(d.additions), 0)::bigint AS total_additions,
    COALESCE(SUM(d.deletions), 0)::bigint AS total_deletions,
    COALESCE(SUM(pc.cost_micros), 0)::bigint AS total_cost_micros,
    COALESCE(SUM(pc.cost_micros) FILTER (WHERE d.pull_request_state = 'merged'), 0)::bigint AS merged_cost_micros
FROM deduped d
JOIN pr_costs pc ON pc.pr_key = d.pr_key;

-- name: GetPRInsightsTimeSeries :many
-- Returns daily PR counts grouped by state for the chart.
-- Uses a CTE to deduplicate by PR URL so that multiple chats referencing
-- the same pull request are only counted once (keeping the most recent chat).
WITH deduped AS (
    SELECT DISTINCT ON (COALESCE(NULLIF(cds.url, ''), c.id::text))
        cds.pull_request_state,
        c.created_at
    FROM chat_diff_statuses cds
    JOIN chats c ON c.id = cds.chat_id
    WHERE cds.pull_request_state IS NOT NULL
      AND c.created_at >= @start_date::timestamptz
      AND c.created_at < @end_date::timestamptz
      AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ORDER BY COALESCE(NULLIF(cds.url, ''), c.id::text), c.created_at DESC, c.id DESC
)
SELECT
    date_trunc('day', created_at)::timestamptz AS date,
    COUNT(*)::bigint AS prs_created,
    COUNT(*) FILTER (WHERE pull_request_state = 'merged')::bigint AS prs_merged,
    COUNT(*) FILTER (WHERE pull_request_state = 'closed')::bigint AS prs_closed
FROM deduped
GROUP BY date_trunc('day', created_at)
ORDER BY date_trunc('day', created_at);

-- name: GetPRInsightsPerModel :many
-- Returns PR metrics grouped by the model used for each chat.
-- Uses two CTEs: pr_costs sums cost for the PR-linked chat and its
-- direct children (that lack their own PR), and deduped picks one row
-- per PR for state/additions/deletions/model (model comes from the
-- most recent chat).
WITH pr_costs AS (
    SELECT
        prc.pr_key,
        COALESCE(SUM(cc.cost_micros), 0) AS cost_micros
    FROM (
        SELECT DISTINCT
            COALESCE(NULLIF(cds.url, ''), c.id::text) AS pr_key,
            related.id AS chat_id
        FROM chat_diff_statuses cds
        JOIN chats c ON c.id = cds.chat_id
        JOIN chats related
          ON related.id = c.id
          OR (related.parent_chat_id = c.id
              AND NOT EXISTS (
                  SELECT 1 FROM chat_diff_statuses cds2
                  WHERE cds2.chat_id = related.id
                    AND cds2.pull_request_state IS NOT NULL
              ))
        WHERE cds.pull_request_state IS NOT NULL
          AND c.created_at >= @start_date::timestamptz
          AND c.created_at < @end_date::timestamptz
          AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ) prc
    LEFT JOIN LATERAL (
        SELECT COALESCE(SUM(cm.total_cost_micros), 0) AS cost_micros
        FROM chat_messages cm
        WHERE cm.chat_id = prc.chat_id
          AND cm.total_cost_micros IS NOT NULL
    ) cc ON TRUE
    GROUP BY prc.pr_key
),
deduped AS (
    SELECT DISTINCT ON (COALESCE(NULLIF(cds.url, ''), c.id::text))
        COALESCE(NULLIF(cds.url, ''), c.id::text) AS pr_key,
        cds.pull_request_state,
        cds.additions,
        cds.deletions,
        cmc.id AS model_config_id,
        cmc.display_name,
        cmc.model,
        cmc.provider
    FROM chat_diff_statuses cds
    JOIN chats c ON c.id = cds.chat_id
    LEFT JOIN chat_model_configs cmc ON cmc.id = c.last_model_config_id
    WHERE cds.pull_request_state IS NOT NULL
      AND c.created_at >= @start_date::timestamptz
      AND c.created_at < @end_date::timestamptz
      AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ORDER BY COALESCE(NULLIF(cds.url, ''), c.id::text), c.created_at DESC, c.id DESC
)
SELECT
    d.model_config_id,
    COALESCE(NULLIF(d.display_name, ''), NULLIF(d.model, ''), 'Unknown')::text AS display_name,
    COALESCE(d.provider, 'unknown')::text AS provider,
    COUNT(*)::bigint AS total_prs,
    COUNT(*) FILTER (WHERE d.pull_request_state = 'merged')::bigint AS merged_prs,
    COALESCE(SUM(d.additions), 0)::bigint AS total_additions,
    COALESCE(SUM(d.deletions), 0)::bigint AS total_deletions,
    COALESCE(SUM(pc.cost_micros), 0)::bigint AS total_cost_micros,
    COALESCE(SUM(pc.cost_micros) FILTER (WHERE d.pull_request_state = 'merged'), 0)::bigint AS merged_cost_micros
FROM deduped d
JOIN pr_costs pc ON pc.pr_key = d.pr_key
GROUP BY d.model_config_id, d.display_name, d.model, d.provider
ORDER BY total_prs DESC;

-- name: GetPRInsightsRecentPRs :many
-- Returns individual PR rows with cost for the recent PRs table.
-- Uses two CTEs: pr_costs sums cost for the PR-linked chat and its
-- direct children (that lack their own PR), and deduped picks one row
-- per PR for metadata.
WITH pr_costs AS (
    SELECT
        prc.pr_key,
        COALESCE(SUM(cc.cost_micros), 0) AS cost_micros
    FROM (
        SELECT DISTINCT
            COALESCE(NULLIF(cds.url, ''), c.id::text) AS pr_key,
            related.id AS chat_id
        FROM chat_diff_statuses cds
        JOIN chats c ON c.id = cds.chat_id
        JOIN chats related
          ON related.id = c.id
          OR (related.parent_chat_id = c.id
              AND NOT EXISTS (
                  SELECT 1 FROM chat_diff_statuses cds2
                  WHERE cds2.chat_id = related.id
                    AND cds2.pull_request_state IS NOT NULL
              ))
        WHERE cds.pull_request_state IS NOT NULL
          AND c.created_at >= @start_date::timestamptz
          AND c.created_at < @end_date::timestamptz
          AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ) prc
    LEFT JOIN LATERAL (
        SELECT COALESCE(SUM(cm.total_cost_micros), 0) AS cost_micros
        FROM chat_messages cm
        WHERE cm.chat_id = prc.chat_id
          AND cm.total_cost_micros IS NOT NULL
    ) cc ON TRUE
    GROUP BY prc.pr_key
),
deduped AS (
    SELECT DISTINCT ON (COALESCE(NULLIF(cds.url, ''), c.id::text))
        COALESCE(NULLIF(cds.url, ''), c.id::text) AS pr_key,
        c.id AS chat_id,
        cds.pull_request_title AS pr_title,
        cds.url AS pr_url,
        cds.pr_number,
        cds.pull_request_state AS state,
        cds.pull_request_draft AS draft,
        cds.additions,
        cds.deletions,
        cds.changed_files,
        cds.commits,
        cds.approved,
        cds.changes_requested,
        cds.reviewer_count,
        cds.author_login,
        cds.author_avatar_url,
        COALESCE(cds.base_branch, '')::text AS base_branch,
        COALESCE(NULLIF(cmc.display_name, ''), NULLIF(cmc.model, ''), 'Unknown')::text AS model_display_name,
        c.created_at
    FROM chat_diff_statuses cds
    JOIN chats c ON c.id = cds.chat_id
    LEFT JOIN chat_model_configs cmc ON cmc.id = c.last_model_config_id
    WHERE cds.pull_request_state IS NOT NULL
      AND c.created_at >= @start_date::timestamptz
      AND c.created_at < @end_date::timestamptz
      AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
    ORDER BY COALESCE(NULLIF(cds.url, ''), c.id::text), c.created_at DESC, c.id DESC
)
SELECT * FROM (
    SELECT
        d.chat_id,
        d.pr_title,
        d.pr_url,
        d.pr_number,
        d.state,
        d.draft,
        d.additions,
        d.deletions,
        d.changed_files,
        d.commits,
        d.approved,
        d.changes_requested,
        d.reviewer_count,
        d.author_login,
        d.author_avatar_url,
        d.base_branch,
        d.model_display_name,
        COALESCE(pc.cost_micros, 0)::bigint AS cost_micros,
        d.created_at
    FROM deduped d
    JOIN pr_costs pc ON pc.pr_key = d.pr_key
) sub
ORDER BY sub.created_at DESC
LIMIT @limit_val::int;
