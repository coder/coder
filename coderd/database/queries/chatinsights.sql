-- PR Insights queries for the /agents analytics dashboard.
-- These aggregate data from chat_diff_statuses (PR metadata) joined
-- with chats and chat_messages (cost) to power the PR Insights view.

-- name: GetPRInsightsSummary :one
-- Returns aggregate PR metrics for the given date range.
-- The handler calls this twice (current + previous period) for trends.
SELECT
    COUNT(*)::bigint AS total_prs_created,
    COUNT(*) FILTER (WHERE cds.pull_request_state = 'merged')::bigint AS total_prs_merged,
    COUNT(*) FILTER (WHERE cds.pull_request_state = 'closed')::bigint AS total_prs_closed,
    COALESCE(SUM(cds.additions), 0)::bigint AS total_additions,
    COALESCE(SUM(cds.deletions), 0)::bigint AS total_deletions,
    COALESCE(SUM(cc.cost_micros), 0)::bigint AS total_cost_micros,
    COALESCE(SUM(cc.cost_micros) FILTER (WHERE cds.pull_request_state = 'merged'), 0)::bigint AS merged_cost_micros
FROM chat_diff_statuses cds
JOIN chats c ON c.id = cds.chat_id
LEFT JOIN (
    SELECT
        COALESCE(ch.root_chat_id, ch.id) AS root_id,
        COALESCE(SUM(cm.total_cost_micros), 0) AS cost_micros
    FROM chat_messages cm
    JOIN chats ch ON ch.id = cm.chat_id
    WHERE cm.total_cost_micros IS NOT NULL
    GROUP BY COALESCE(ch.root_chat_id, ch.id)
) cc ON cc.root_id = COALESCE(c.root_chat_id, c.id)
WHERE cds.pull_request_state IS NOT NULL
  AND c.created_at >= @start_date::timestamptz
  AND c.created_at < @end_date::timestamptz
  AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid);

-- name: GetPRInsightsTimeSeries :many
-- Returns daily PR counts grouped by state for the chart.
SELECT
    date_trunc('day', c.created_at)::timestamptz AS date,
    COUNT(*)::bigint AS prs_created,
    COUNT(*) FILTER (WHERE cds.pull_request_state = 'merged')::bigint AS prs_merged,
    COUNT(*) FILTER (WHERE cds.pull_request_state = 'closed')::bigint AS prs_closed
FROM chat_diff_statuses cds
JOIN chats c ON c.id = cds.chat_id
WHERE cds.pull_request_state IS NOT NULL
  AND c.created_at >= @start_date::timestamptz
  AND c.created_at < @end_date::timestamptz
  AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
GROUP BY date_trunc('day', c.created_at)
ORDER BY date_trunc('day', c.created_at);

-- name: GetPRInsightsPerModel :many
-- Returns PR metrics grouped by the model used for each chat.
SELECT
    cmc.id AS model_config_id,
    cmc.display_name,
    cmc.provider,
    COUNT(*)::bigint AS total_prs,
    COUNT(*) FILTER (WHERE cds.pull_request_state = 'merged')::bigint AS merged_prs,
    COALESCE(SUM(cds.additions), 0)::bigint AS total_additions,
    COALESCE(SUM(cds.deletions), 0)::bigint AS total_deletions,
    COALESCE(SUM(cc.cost_micros), 0)::bigint AS total_cost_micros,
    COALESCE(SUM(cc.cost_micros) FILTER (WHERE cds.pull_request_state = 'merged'), 0)::bigint AS merged_cost_micros
FROM chat_diff_statuses cds
JOIN chats c ON c.id = cds.chat_id
JOIN chat_model_configs cmc ON cmc.id = c.last_model_config_id
LEFT JOIN (
    SELECT
        COALESCE(ch.root_chat_id, ch.id) AS root_id,
        COALESCE(SUM(cm.total_cost_micros), 0) AS cost_micros
    FROM chat_messages cm
    JOIN chats ch ON ch.id = cm.chat_id
    WHERE cm.total_cost_micros IS NOT NULL
    GROUP BY COALESCE(ch.root_chat_id, ch.id)
) cc ON cc.root_id = COALESCE(c.root_chat_id, c.id)
WHERE cds.pull_request_state IS NOT NULL
  AND c.created_at >= @start_date::timestamptz
  AND c.created_at < @end_date::timestamptz
  AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
GROUP BY cmc.id, cmc.display_name, cmc.provider
ORDER BY total_prs DESC;

-- name: GetPRInsightsRecentPRs :many
-- Returns individual PR rows with cost for the recent PRs table.
SELECT
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
    COALESCE(cmc.display_name, cmc.model)::text AS model_display_name,
    COALESCE(cc.cost_micros, 0)::bigint AS cost_micros,
    c.created_at
FROM chat_diff_statuses cds
JOIN chats c ON c.id = cds.chat_id
JOIN chat_model_configs cmc ON cmc.id = c.last_model_config_id
LEFT JOIN (
    SELECT
        COALESCE(ch.root_chat_id, ch.id) AS root_id,
        COALESCE(SUM(cm.total_cost_micros), 0) AS cost_micros
    FROM chat_messages cm
    JOIN chats ch ON ch.id = cm.chat_id
    WHERE cm.total_cost_micros IS NOT NULL
    GROUP BY COALESCE(ch.root_chat_id, ch.id)
) cc ON cc.root_id = COALESCE(c.root_chat_id, c.id)
WHERE cds.pull_request_state IS NOT NULL
  AND c.created_at >= @start_date::timestamptz
  AND c.created_at < @end_date::timestamptz
  AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id')::uuid)
ORDER BY c.created_at DESC
LIMIT @limit_val::int;
