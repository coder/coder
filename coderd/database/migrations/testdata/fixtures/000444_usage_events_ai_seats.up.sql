INSERT INTO usage_events (
    id,
    event_type,
    event_data,
    created_at,
    publish_started_at,
    published_at,
    failure_message
)
VALUES
-- Unpublished hb_ai_seats_v1 event.
(
    'ai-seats-event1',
    'hb_ai_seats_v1',
    '{"count":3}',
    '2023-06-01 00:00:00+00',
    NULL,
    NULL,
    NULL
);
