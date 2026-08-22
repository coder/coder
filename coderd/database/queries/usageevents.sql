-- name: InsertUsageEvent :exec
-- Duplicate events are ignored intentionally to allow for multiple replicas to
-- publish heartbeat events.
INSERT INTO
    usage_events (
        id,
        event_type,
        event_data,
        created_at,
        publish_started_at,
        published_at,
        failure_message
    )
VALUES
    (@id, @event_type, @event_data, @created_at, NULL, NULL, NULL)
ON CONFLICT (id) DO NOTHING;

-- name: UsageEventExistsByID :one
SELECT EXISTS(
    SELECT 1 FROM usage_events WHERE id = @id
)::bool;

-- name: ListUsageEventCreatedAtsByTypeSince :many
-- Used by the usage generator to find missing heartbeat buckets.
SELECT created_at
FROM usage_events
WHERE event_type = @event_type
  AND created_at >= @since::timestamptz;

-- name: SelectUsageEventsForPublishing :many
WITH usage_events AS (
    UPDATE
        usage_events
    SET
        publish_started_at = @now::timestamptz
    WHERE
        id IN (
            SELECT
                potential_event.id
            FROM
                usage_events potential_event
            WHERE
                -- Do not publish events that have already been published or
                -- have permanently failed to publish.
                potential_event.published_at IS NULL
                -- Do not publish events that are already being published by
                -- another replica.
                AND (
                    potential_event.publish_started_at IS NULL
                    -- If the event has publish_started_at set, it must be older
                    -- than an hour ago. This is so we can retry publishing
                    -- events where the replica exited or couldn't update the
                    -- row.
                    -- The parentheses around @now::timestamptz are necessary to
                    -- avoid sqlc from generating an extra argument.
                    OR potential_event.publish_started_at < (@now::timestamptz) - INTERVAL '1 hour'
                )
                -- Do not publish events older than 30 days. Tallyman will
                -- always permanently reject these events anyways. This is to
                -- avoid duplicate events being billed to customers, as
                -- Metronome will only deduplicate events within 34 days.
                -- Also, the same parentheses thing here as above.
                AND potential_event.created_at > (@now::timestamptz) - INTERVAL '30 days'
            ORDER BY potential_event.created_at ASC
            FOR UPDATE SKIP LOCKED
            LIMIT 100
        )
    RETURNING *
)
SELECT *
-- Note that this selects from the CTE, not the original table. The CTE is named
-- the same as the original table to trick sqlc into reusing the existing struct
-- for the table.
FROM usage_events
-- The CTE and the reorder is required because UPDATE doesn't guarantee order.
ORDER BY created_at ASC;

-- name: UpdateUsageEventsPostPublish :exec
UPDATE
    usage_events
SET
    publish_started_at = NULL,
    published_at = CASE WHEN input.set_published_at THEN @now::timestamptz ELSE NULL END,
    failure_message = NULLIF(input.failure_message, '')
FROM (
    SELECT
        UNNEST(@ids::text[]) AS id,
        UNNEST(@failure_messages::text[]) AS failure_message,
        UNNEST(@set_published_ats::boolean[]) AS set_published_at
) input
WHERE
    input.id = usage_events.id
    -- If the number of ids, failure messages, and set published ats are not the
    -- same, do not do anything. Unfortunately you can't really throw from a
    -- query without writing a function or doing some jank like dividing by
    -- zero, so this is the best we can do.
    AND cardinality(@ids::text[]) = cardinality(@failure_messages::text[])
    AND cardinality(@ids::text[]) = cardinality(@set_published_ats::boolean[]);

-- name: GetUsageEventsStats :one
-- Read-only stats about unpublished usage events, used for Prometheus metrics
-- in the usage publisher. Events older than 30 days will never be published
-- (Tallyman would permanently reject them), so they are counted separately as
-- "expired".
SELECT
    -- The parentheses around @now::timestamptz are necessary to avoid sqlc
    -- from generating an extra argument.
    (COUNT(*) FILTER (WHERE created_at > (@now::timestamptz) - INTERVAL '30 days'))::bigint AS pending_count,
    -- COALESCE to the Go zero time value when there are no pending events so
    -- sqlc generates a non-nullable time.Time.
    COALESCE(MIN(created_at) FILTER (WHERE created_at > (@now::timestamptz) - INTERVAL '30 days'), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS oldest_pending_created_at,
    (COUNT(*) FILTER (WHERE created_at <= (@now::timestamptz) - INTERVAL '30 days'))::bigint AS expired_count
FROM
    usage_events
WHERE
    published_at IS NULL;

-- name: GetTotalUsageDCManagedAgentsV1 :one
-- Gets the total number of managed agents created between two dates. Uses the
-- aggregate table to avoid large scans or a complex index on the usage_events
-- table.
--
-- This has the trade off that we can't count accurately between two exact
-- timestamps. The provided timestamps will be converted to UTC and truncated to
-- the events that happened on and between the two dates. Both dates are
-- inclusive.
SELECT
    -- The first cast is necessary since you can't sum strings, and the second
    -- cast is necessary to make sqlc happy.
    COALESCE(SUM((usage_data->>'count')::bigint), 0)::bigint AS total_count
FROM
    usage_events_daily
WHERE
    event_type = 'dc_managed_agents_v1'
    -- Parentheses are necessary to avoid sqlc from generating an extra
    -- argument.
    AND day BETWEEN date_trunc('day', (@start_date::timestamptz) AT TIME ZONE 'UTC')::date AND date_trunc('day', (@end_date::timestamptz) AT TIME ZONE 'UTC')::date;

-- name: GetTotalUsageHBAgentRuntimeV1 :one
-- Gets the total Coder Agent runtime in milliseconds between two timestamps.
-- The start bound is inclusive and the end bound is exclusive.
--
-- Unlike GetTotalUsageDCManagedAgentsV1 this reads usage_events directly
-- rather than the usage_events_daily rollup: hb_agent_runtime_v1 is exactly
-- one row per hourly bucket deployment-wide, with created_at at the bucket
-- start, enforced by the unique partial index
-- idx_usage_events_agent_runtime (which also keeps SUM from counting a
-- bucket twice and serves this query). The result is bucket-granular: a
-- bucket counts entirely against the period containing its start. See
-- enterprise/coderd/usage/generator.go for what a bucket holds. If a
-- usage_events retention policy ever lands, this must move to the daily
-- rollup and accept day-granularity bounds.
SELECT
    -- The first cast is necessary since you can't sum strings, and the second
    -- cast is necessary to make sqlc happy.
    COALESCE(SUM((event_data->>'runtime_ms')::bigint), 0)::bigint AS total_runtime_ms
FROM
    usage_events
WHERE
    event_type = 'hb_agent_runtime_v1'
    AND created_at >= @start_time::timestamptz
    AND created_at < @end_time::timestamptz;
