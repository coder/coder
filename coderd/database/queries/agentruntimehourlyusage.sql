-- name: GetAgentRuntimeHourlyUsage :many
-- Computes one bucket of agent_runtime_hourly_usage from chat messages: the
-- runtime per chat organization and owner, with the owner's current
-- effective Agent Hours group. Like the hb_agent_runtime_v1 payload, it
-- counts soft-deleted messages and messages from all chats.
WITH chat_runtime AS (
    SELECT
        cm.chat_id,
        SUM(cm.runtime_ms)::bigint AS runtime_ms
    FROM chat_messages cm
    WHERE cm.created_at >= @start_time::timestamptz
      AND cm.created_at < @end_time::timestamptz
      AND cm.runtime_ms IS NOT NULL
    GROUP BY cm.chat_id
)
SELECT
    c.organization_id,
    agent_hours_effective_group_id(c.organization_id, c.owner_id)::uuid AS group_id,
    c.owner_id AS user_id,
    SUM(cr.runtime_ms)::bigint AS runtime_ms
FROM chat_runtime cr
JOIN chats c ON c.id = cr.chat_id
GROUP BY c.organization_id, c.owner_id
-- Owners whose runtime sums to zero add nothing to the bucket.
HAVING SUM(cr.runtime_ms) <> 0;

-- name: InsertAgentRuntimeHourlyUsage :exec
INSERT INTO agent_runtime_hourly_usage (
    bucket_start,
    organization_id,
    group_id,
    user_id,
    runtime_ms
)
SELECT
    @bucket_start::timestamptz,
    unnest(@organization_ids::uuid[]),
    unnest(@group_ids::uuid[]),
    unnest(@user_ids::uuid[]),
    unnest(@runtime_ms::bigint[]);
