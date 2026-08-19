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

-- name: UpsertUsageEventsPublishFailures :exec
-- Records publish failures for the given usage event IDs. Rows are copied
-- from usage_events so a stalled replica cannot record state that
-- contradicts the database: only events still unpublished and within the
-- publisher's 30-day selection window (window_start is now minus 30 days;
-- older events are never published) gain a failure row. Bounded by the
-- caller's ID list (at most one publish batch) and served by the primary
-- key. This is only needed when the post-publish update itself failed:
-- ordinarily the schema triggers on usage_events maintain the relation
-- (trigger_record_usage_events_publish_failure records temporary failures
-- and trigger_record_usage_events_publish_outcome resolves published
-- events), so every writer participates, including replicas running an
-- older release during a rolling upgrade.
INSERT INTO usage_events_publish_failures (event_id, inserted_at, created_at)
SELECT id, inserted_at, created_at
FROM usage_events
WHERE
    id = ANY(@ids::text[])
    AND published_at IS NULL
    AND created_at > @window_start::timestamptz
ON CONFLICT (event_id) DO NOTHING;

-- name: PruneUsageEventsPublishFailures :exec
-- Deletes failure rows whose events aged out of the publisher's 30-day
-- selection window: they can never be published, so they no longer count
-- as pending failures. The LIMIT bounds each prune; the publisher runs one
-- per cycle, and rows age out no faster than they were once inserted, so
-- pruning keeps up.
DELETE FROM usage_events_publish_failures
WHERE event_id IN (
    SELECT event_id
    FROM usage_events_publish_failures
    WHERE created_at <= @window_start::timestamptz
    ORDER BY created_at ASC
    LIMIT 1000
);

-- name: PruneUsageEventsPublishRejections :exec
-- Deletes rejection rows older than the failure threshold: they no longer
-- count as recent rejections, so keeping them only grows the relation. The
-- LIMIT bounds each prune; the publisher runs one per cycle, and rows age
-- past the threshold no faster than they were once inserted, so pruning
-- keeps up.
DELETE FROM usage_events_publish_rejections
WHERE event_id IN (
    SELECT event_id
    FROM usage_events_publish_rejections
    WHERE published_at <= @rejected_before::timestamptz
    ORDER BY published_at ASC
    LIMIT 1000
);

-- name: GetUsagePublishStatus :one
-- Returns the status of usage event publishing so callers can detect publish
-- failures. NULL results are encoded as the zero timestamp because sqlc
-- cannot reliably infer the nullability of aggregate expressions. All cutoff
-- parameters are computed by the caller so tests can control time:
--   - enabled_since: when usage publishing most recently became enabled, as
--     tracked by the entitlements refresh in a runtime config key. An
--     event's effective stuck time is clamped to this, so a backlog
--     accumulated while publishing was disabled (or before it was first
--     enabled, e.g. on an air-gapped deployment that switches licenses)
--     gets the full failure threshold as a grace period after
--     (re-)enablement. Continuous license renewals do not advance it, so an
--     ongoing outage keeps warning across renewals.
--   - window_start: the start of the publisher's selection window (now minus
--     30 days, matching SelectUsageEventsForPublishing). Events older than
--     this are never published, so they must not trigger a failure forever.
--   - stuck_cutoff: now minus the failure threshold. Events at the front of
--     the publisher's queue whose effective stuck time is before this are
--     considered stuck. Stuckness is measured against inserted_at rather
--     than created_at because heartbeat events backfilled after downtime
--     carry a historical created_at; measuring event age would flag them as
--     failing before publishing was ever attempted. Insertion age keeps the
--     effective stuck time monotonic per row: a failed attempt cannot
--     replace an already-breached insertion age with a newer timestamp and
--     flap the warning off for another threshold, and whatever kept a row
--     unattempted past the threshold (publisher outage, displacement behind
--     failing rows) is itself a publish failure.
--   - attempt_expired_before: now minus the publisher's 1-hour attempt
--     expiry (matching SelectUsageEventsForPublishing). In-flight attempts
--     newer than this are skipped; older markers are from replicas that
--     exited mid-publish, and the publisher considers those rows retryable,
--     so the status scan must too or they could stay stuck without warning.
--     Events with an active publish failure need no queue-position
--     visibility here: schema triggers on usage_events record them in
--     usage_events_publish_failures, which the failing CTE below reads
--     without scanning the backlog for failed rows.
--   - rejected_after: now minus the failure threshold. Permanent rejections
--     that happened after this and within the current enabled period are
--     considered recent failures; rejections from a prior enabled period
--     must not raise a warning before the re-enabled publisher has attempted
--     anything.
WITH last_success AS (
    -- The latest successful publish among the most recent publish outcomes.
    -- Rows with a failure_message and a published_at are permanent
    -- rejections, not successes. The probe inspects at most one publisher
    -- batch of the newest published rows, which
    -- idx_usage_events_select_for_publishing's leading published_at column
    -- delivers in order, instead of walking backward through an unbounded
    -- rejection streak to find an older success; a success older than the
    -- most recent 100 outcomes reports as no recent success, which the
    -- ongoing rejections make moot.
    SELECT MAX(published_at) AS last_published_at
    FROM (
        SELECT published_at, failure_message
        FROM usage_events
        WHERE published_at IS NOT NULL
        ORDER BY published_at DESC
        LIMIT 100
    ) recent
    WHERE failure_message IS NULL
), stuck AS (
    -- The earliest effective stuck time among events at the front of the
    -- publisher's queue. This deliberately inspects at most one publisher
    -- batch per branch (100 rows, matching SelectUsageEventsForPublishing)
    -- instead of computing an exact minimum over a potentially unbounded
    -- unpublished backlog (e.g. after publishing was disabled for weeks). A
    -- stuck event behind the first batch surfaces once the queue ahead of
    -- it drains or is attempted; the publisher cannot reach it any earlier
    -- either, and active failures are covered independently by the
    -- publisher's failure-streak marker (see attempt_expired_before doc).
    -- Two branches, each separately index-served by
    -- idx_usage_events_select_for_publishing (published_at,
    -- publish_started_at, created_at): the not-in-flight branch pins both
    -- leading columns, letting the index deliver created_at order and stop
    -- at the LIMIT, while the expired-attempt branch (replicas that exited
    -- mid-publish, which the publisher considers retryable, so the status
    -- scan must too) ranges over publish_started_at and sorts its naturally
    -- tiny row set. A single OR'd scan could not use the index for the
    -- global created_at order and would top-N sort the whole backlog.
    SELECT MIN(
        -- The clamp to enabled_since grants the post-(re-)enablement grace
        -- period; see the enabled_since parameter doc.
        GREATEST(queued.inserted_at, @enabled_since::timestamptz)
    ) AS oldest_stuck_at
    FROM (
        (
            SELECT inserted_at
            FROM usage_events
            WHERE published_at IS NULL
                AND publish_started_at IS NULL
                AND created_at > @window_start::timestamptz
            ORDER BY created_at ASC
            LIMIT 100
        )
        UNION ALL
        (
            SELECT inserted_at
            FROM usage_events
            WHERE published_at IS NULL
                AND publish_started_at IS NOT NULL
                AND publish_started_at < @attempt_expired_before::timestamptz
                AND created_at > @window_start::timestamptz
            ORDER BY created_at ASC
            LIMIT 100
        )
    ) queued
), failing AS (
    -- The oldest event with an active publish failure, from the dedicated
    -- failures relation the publisher maintains with each batch outcome
    -- (see usage_events_publish_failures). Failing events need this
    -- separate probe because the queue-front branches above cannot see
    -- them while a fresh retry holds them in flight or newly inserted
    -- backfills with older created_at displace them. Index-served: the
    -- probe walks idx_usage_events_publish_failures_inserted_at from the
    -- oldest entry, and pruning keeps rows past the window rare, so the
    -- walk stops almost immediately. The enabled_since clamp grants the
    -- post-(re-)enablement grace period as in the stuck CTE.
    SELECT GREATEST(inserted_at, @enabled_since::timestamptz) AS oldest_failing_at
    FROM usage_events_publish_failures
    WHERE created_at > @window_start::timestamptz
    ORDER BY inserted_at ASC
    LIMIT 1
)
SELECT
    COALESCE((SELECT last_published_at FROM last_success), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS last_published_at,
    COALESCE((
        SELECT MIN(oldest) FROM (
            SELECT oldest_stuck_at AS oldest FROM stuck
            UNION ALL
            SELECT oldest_failing_at AS oldest FROM failing
        ) candidates
        WHERE oldest < @stuck_cutoff::timestamptz
    ), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS oldest_stuck_at,
    -- The earliest recent permanent rejection, from the dedicated
    -- rejections relation the publish-outcome trigger maintains (see
    -- usage_events_publish_rejections). Reading it here instead of
    -- usage_events keeps the probe off the successful-publish workload:
    -- the relation holds only rejections, pruning keeps it to the recent
    -- ones, and idx_usage_events_publish_rejections_published_at serves
    -- the range.
    COALESCE((
        SELECT MIN(published_at)
        FROM usage_events_publish_rejections
        WHERE published_at > @rejected_after::timestamptz
            AND published_at >= @enabled_since::timestamptz
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
