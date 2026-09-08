-- name: AccountAgentTimeMessages :one
SELECT account_agent_time_messages(@message_ids::bigint[])::bigint;

-- name: BackfillAgentTimeBatch :one
WITH candidate_chats AS MATERIALIZED (
    SELECT c.id
    FROM chats c
    WHERE c.organization_id = @organization_id::uuid
      AND EXISTS (
          SELECT 1
          FROM chat_messages cm
          LEFT JOIN chat_message_agent_time_accounted accounted ON accounted.message_id = cm.id
          WHERE cm.chat_id = c.id
            AND cm.runtime_ms IS NOT NULL
            AND accounted.message_id IS NULL
            AND cm.id > @cursor_message_id::bigint
      )
    ORDER BY c.id ASC
    LIMIT @limit_count::int
    FOR UPDATE OF c SKIP LOCKED
),
candidate_messages AS MATERIALIZED (
    SELECT cm.id
    FROM chat_messages cm
    JOIN candidate_chats c ON c.id = cm.chat_id
    LEFT JOIN chat_message_agent_time_accounted accounted ON accounted.message_id = cm.id
    WHERE cm.runtime_ms IS NOT NULL
      AND accounted.message_id IS NULL
      AND cm.id > @cursor_message_id::bigint
    ORDER BY cm.id ASC
    LIMIT @limit_count::int
),
accounted AS (
    SELECT account_agent_time_messages(ARRAY(
        SELECT id
        FROM candidate_messages
        ORDER BY id
    ))::bigint AS processed_messages
)
SELECT
    COALESCE((SELECT MAX(id) FROM candidate_messages), @cursor_message_id::bigint)::bigint AS cursor_message_id,
    COALESCE((SELECT COUNT(*) FROM candidate_messages), 0)::bigint AS selected_messages,
    COALESCE((SELECT processed_messages FROM accounted), 0)::bigint AS processed_messages;

-- name: HasUnaccountedAgentTimeMessages :one
SELECT EXISTS (
    SELECT 1
    FROM chat_messages cm
    JOIN chats c ON c.id = cm.chat_id
    LEFT JOIN chat_message_agent_time_accounted accounted ON accounted.message_id = cm.id
    WHERE c.organization_id = @organization_id::uuid
      AND cm.runtime_ms IS NOT NULL
      AND accounted.message_id IS NULL
    LIMIT 1
)::boolean;

-- name: EnsureAgentTimeBackfillStatuses :execrows
INSERT INTO agent_time_backfill_status (organization_id)
SELECT organizations.id
FROM organizations
UNION
SELECT organization_id FROM agent_time_organization_daily
ON CONFLICT (organization_id) DO NOTHING;

-- name: AcquireAgentTimeBackfillOrganization :one
SELECT organization_id, cursor_message_id, processed_messages, completed_at, last_error, last_error_at, updated_at
FROM agent_time_backfill_status
WHERE completed_at IS NULL
ORDER BY updated_at ASC, organization_id ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: UpdateAgentTimeBackfillProgress :exec
UPDATE agent_time_backfill_status
SET cursor_message_id = @cursor_message_id::bigint,
    processed_messages = processed_messages + @processed_messages::bigint,
    last_error = '',
    last_error_at = NULL,
    updated_at = now()
WHERE organization_id = @organization_id::uuid;

-- name: ResetAgentTimeBackfillCursor :exec
UPDATE agent_time_backfill_status
SET cursor_message_id = 0,
    last_error = '',
    last_error_at = NULL,
    updated_at = now()
WHERE organization_id = @organization_id::uuid;

-- name: CompleteAgentTimeBackfillOrganization :exec
UPDATE agent_time_backfill_status
SET completed_at = now(),
    last_error = '',
    last_error_at = NULL,
    updated_at = now()
WHERE organization_id = @organization_id::uuid;

-- name: MarkAgentTimeBackfillFailed :exec
-- Status discovery can roll back with the first batch. Persist the failure even
-- when that transaction created the organization's first status row.
INSERT INTO agent_time_backfill_status (organization_id, last_error, last_error_at, updated_at)
VALUES (@organization_id::uuid, @last_error::text, now(), now())
ON CONFLICT (organization_id) DO UPDATE SET
    last_error = EXCLUDED.last_error,
    last_error_at = EXCLUDED.last_error_at,
    updated_at = EXCLUDED.updated_at;

-- name: GetAgentTimeStatus :one
WITH scopes AS (
    SELECT id AS organization_id FROM organizations
    UNION
    SELECT organization_id FROM agent_time_backfill_status
    UNION
    SELECT @organization_id::uuid WHERE @organization_id::uuid <> '00000000-0000-0000-0000-000000000000'
), selected_scopes AS (
    SELECT organization_id FROM scopes
    WHERE @organization_id::uuid = '00000000-0000-0000-0000-000000000000'
        OR organization_id = @organization_id
)
SELECT
    capture.capture_started_at,
    BOOL_AND(scope.organization_id IS NULL OR status.completed_at IS NOT NULL)::boolean AS backfill_complete,
    COALESCE(MIN(NULLIF(status.last_error, '')), '')::text AS backfill_error,
    COALESCE(SUM(status.processed_messages), 0)::bigint AS processed_messages,
    CASE
        WHEN capture.capture_started_at = date_trunc('day', capture.capture_started_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
            THEN (capture.capture_started_at AT TIME ZONE 'UTC')::date
        ELSE ((capture.capture_started_at AT TIME ZONE 'UTC')::date + 1)
    END::date AS earliest_date
FROM agent_time_capture capture
LEFT JOIN selected_scopes scope ON TRUE
LEFT JOIN agent_time_backfill_status status ON status.organization_id = scope.organization_id
WHERE capture.id = 1
GROUP BY capture.capture_started_at;
