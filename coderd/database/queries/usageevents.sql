-- name: InsertUsageEvent :exec
-- Duplicate events are ignored intentionally to allow for multiple replicas to
-- publish heartbeat events.
INSERT INTO
    usage_events (
        id,
        event_type,
        event_data,
        created_at,
        inserted_at,
        publish_started_at,
        published_at,
        failure_message
    )
VALUES
    (@id, @event_type, @event_data, @created_at, @inserted_at, NULL, NULL, NULL)
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

-- name: GetUsagePublishStatus :one
-- Returns the status of usage event publishing so callers can detect publish
-- failures. NULL results are encoded as the zero timestamp because sqlc
-- cannot reliably infer the nullability of aggregate expressions. All cutoff
-- parameters are computed by the caller so tests can control time:
--   - license_start: the nbf of the earliest currently-valid license with
--     usage publishing enabled. Events that have never had a publish attempt
--     count as stuck only from license_start, giving the publisher a grace
--     period to work through a backlog accumulated while publishing was
--     disabled or before it was first enabled (e.g. after switching from an
--     air-gapped license). Events with at least one failed attempt (an
--     unpublished row's failure_message is set by every failed attempt)
--     count from their insertion time regardless, so an ongoing outage keeps
--     warning even though a license renewal advances license_start.
--   - window_start: the start of the publisher's selection window (now minus
--     30 days, matching SelectUsageEventsForPublishing). Events older than
--     this are never published, so they must not trigger a failure forever.
--   - stuck_cutoff: now minus the failure threshold. Unpublished events
--     whose effective stuck time is before this are considered stuck.
--     Stuckness is measured against inserted_at rather than created_at
--     because heartbeat events backfilled after downtime carry a historical
--     created_at; measuring event age would flag them as failing before
--     publishing was ever attempted.
--   - rejected_after: now minus the failure threshold. Permanent rejections
--     that happened after this are considered recent failures.
WITH last_success AS (
    -- The latest successful publish. Rows with a failure_message and a
    -- published_at are permanent rejections, not successes.
    SELECT MAX(published_at) AS last_published_at
    FROM usage_events
    WHERE published_at IS NOT NULL
        AND failure_message IS NULL
), never_attempted AS (
    -- The oldest insertion time among unpublished events with no publish
    -- attempt yet. Bounded by idx_usage_events_unpublished_no_attempt.
    SELECT MIN(inserted_at) AS min_inserted_at
    FROM usage_events
    WHERE published_at IS NULL
        AND failure_message IS NULL
        AND created_at > @window_start::timestamptz
), attempted AS (
    -- The oldest insertion time among unpublished events with at least one
    -- failed attempt. Bounded by idx_usage_events_unpublished_attempted.
    SELECT MIN(inserted_at) AS min_inserted_at
    FROM usage_events
    WHERE published_at IS NULL
        AND failure_message IS NOT NULL
        AND created_at > @window_start::timestamptz
), stuck AS (
    -- The effective stuck time of the oldest event that should have been
    -- published by now but wasn't. GREATEST is monotonic in inserted_at, so
    -- applying it to each branch's MIN is equivalent to taking MIN over
    -- per-event effective stuck times, without scanning the whole backlog.
    -- The CASE guards the never-attempted branch because GREATEST ignores
    -- NULL arguments: no rows must yield NULL, not license_start. LEAST
    -- likewise ignores a NULL branch.
    SELECT LEAST(
        CASE
            WHEN never_attempted.min_inserted_at IS NOT NULL
                THEN GREATEST(never_attempted.min_inserted_at, @license_start::timestamptz)
        END,
        attempted.min_inserted_at
    ) AS oldest_stuck_at
    FROM never_attempted, attempted
)
SELECT
    COALESCE((SELECT last_published_at FROM last_success), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS last_published_at,
    COALESCE((SELECT oldest_stuck_at FROM stuck WHERE oldest_stuck_at < @stuck_cutoff::timestamptz), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS oldest_stuck_at,
    -- The earliest recent permanent rejection.
    COALESCE((
        SELECT MIN(published_at)
        FROM usage_events
        WHERE published_at IS NOT NULL
            AND failure_message IS NOT NULL
            AND published_at > @rejected_after::timestamptz
    ), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS earliest_recent_rejection_at;

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
