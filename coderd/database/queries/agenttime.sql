-- name: GetAgentTimeEarliestDate :one
WITH daily AS (
    SELECT organization_id, '00000000-0000-0000-0000-000000000000'::uuid AS user_id, day, agent_time_ms
    FROM agent_time_organization_daily
    WHERE @use_organization_summary::boolean
    UNION ALL
    SELECT organization_id, user_id, day, agent_time_ms::numeric
    FROM agent_time_daily
    WHERE NOT @use_organization_summary::boolean
)
SELECT COALESCE(MIN(day)::text, '')::text AS earliest_date
FROM daily
WHERE (@organization_id::uuid = '00000000-0000-0000-0000-000000000000' OR organization_id = @organization_id)
    AND (@user_id::uuid = '00000000-0000-0000-0000-000000000000' OR user_id = @user_id);

-- name: GetAgentTimeSummary :one
WITH daily AS (
    SELECT organization_id, '00000000-0000-0000-0000-000000000000'::uuid AS user_id, day, agent_time_ms
    FROM agent_time_organization_daily
    WHERE @use_organization_summary::boolean
    UNION ALL
    SELECT organization_id, user_id, day, agent_time_ms::numeric
    FROM agent_time_daily
    WHERE NOT @use_organization_summary::boolean
)
SELECT COALESCE(SUM(agent_time_ms), 0)::text AS agent_time_ms,
    COUNT(DISTINCT CASE WHEN @group_by::text = 'user' THEN user_id ELSE organization_id END)::bigint AS count
FROM daily
WHERE day >= @start_date::date AND day < @end_date::date
    AND (@organization_id::uuid = '00000000-0000-0000-0000-000000000000' OR organization_id = @organization_id)
    AND (@user_id::uuid = '00000000-0000-0000-0000-000000000000' OR user_id = @user_id);

-- name: GetAgentTimeBuckets :many
WITH daily AS (
    SELECT organization_id, '00000000-0000-0000-0000-000000000000'::uuid AS user_id, day, agent_time_ms
    FROM agent_time_organization_daily
    WHERE @use_organization_summary::boolean
    UNION ALL
    SELECT organization_id, user_id, day, agent_time_ms::numeric
    FROM agent_time_daily
    WHERE NOT @use_organization_summary::boolean
)
SELECT date_trunc(@interval::text, day::timestamp)::date::text AS bucket_date,
    SUM(agent_time_ms)::text AS agent_time_ms
FROM daily
WHERE day >= @start_date::date AND day < @end_date::date
    AND (@organization_id::uuid = '00000000-0000-0000-0000-000000000000' OR organization_id = @organization_id)
    AND (@user_id::uuid = '00000000-0000-0000-0000-000000000000' OR user_id = @user_id)
GROUP BY 1
ORDER BY 1;

-- name: GetAgentTimeBreakdown :many
WITH daily AS (
    SELECT organization_id, '00000000-0000-0000-0000-000000000000'::uuid AS user_id, day, agent_time_ms
    FROM agent_time_organization_daily
    WHERE @use_organization_summary::boolean
    UNION ALL
    SELECT organization_id, user_id, day, agent_time_ms::numeric
    FROM agent_time_daily
    WHERE NOT @use_organization_summary::boolean
), totals AS (
    SELECT CASE WHEN @group_by::text = 'user' THEN user_id ELSE organization_id END AS id,
        SUM(agent_time_ms) AS agent_time_ms
    FROM daily
    WHERE day >= @start_date::date AND day < @end_date::date
    AND (@organization_id::uuid = '00000000-0000-0000-0000-000000000000' OR organization_id = @organization_id)
    AND (@user_id::uuid = '00000000-0000-0000-0000-000000000000' OR user_id = @user_id)
    GROUP BY 1
), named AS (
    SELECT totals.id, totals.agent_time_ms,
        CASE WHEN @group_by::text = 'user' THEN
            CASE WHEN u.id IS NULL OR u.deleted THEN 'Deleted user' ELSE u.username END
        ELSE
            CASE WHEN o.id IS NULL OR o.deleted THEN 'Deleted organization'
                ELSE COALESCE(NULLIF(o.display_name, ''), o.name) END
        END::text AS name,
        CASE WHEN @group_by::text = 'user' THEN u.id IS NULL OR u.deleted
            ELSE o.id IS NULL OR o.deleted END::boolean AS deleted
    FROM totals
    LEFT JOIN users u ON @group_by::text = 'user' AND u.id = totals.id
    LEFT JOIN organizations o ON @group_by::text = 'organization' AND o.id = totals.id
)
SELECT id::uuid AS id, name, deleted, agent_time_ms::text AS agent_time_ms FROM named
ORDER BY
    CASE WHEN @sort_by::text = 'agent_time' AND @sort_order::text = 'desc' THEN agent_time_ms END DESC,
    CASE WHEN @sort_by::text = 'agent_time' AND @sort_order::text = 'asc' THEN agent_time_ms END ASC,
    CASE WHEN @sort_by::text = 'name' AND @sort_order::text = 'desc' THEN name END DESC,
    CASE WHEN @sort_by::text = 'name' AND @sort_order::text = 'asc' THEN name END ASC,
    id ASC
LIMIT @page_limit::int OFFSET @page_offset::int;
