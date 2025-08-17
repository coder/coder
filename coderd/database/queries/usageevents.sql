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
                    -- The parenthesis around @now::timestamptz are necessary to
                    -- avoid sqlc from generating an extra argument.
                    OR potential_event.publish_started_at < (@now::timestamptz) - INTERVAL '1 hour'
                )
                -- Do not publish events older than 30 days. Tallyman will
                -- always permanently reject these events anyways. This is to
                -- avoid duplicate events being billed to customers, as
                -- Metronome will only deduplicate events within 34 days.
                -- Also, the same parenthesis thing here as above.
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
