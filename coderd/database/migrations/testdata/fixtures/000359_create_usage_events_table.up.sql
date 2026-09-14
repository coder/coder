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
-- Unpublished dc_managed_agents_v1 event.
(
    'event1',
    'dc_managed_agents_v1',
    '{"count":1}',
    '2023-01-01 00:00:00+00',
    NULL,
    NULL,
    NULL
),
-- Successfully published dc_managed_agents_v1 event.
(
    'event2',
    'dc_managed_agents_v1',
    '{"count":2}',
    '2023-01-01 00:00:00+00',
    NULL,
    '2023-01-01 00:00:02+00',
    NULL
),
-- Publish in progress dc_managed_agents_v1 event.
(
    'event3',
    'dc_managed_agents_v1',
    '{"count":3}',
    '2023-01-01 00:00:00+00',
    '2023-01-01 00:00:01+00',
    NULL,
    NULL
),
-- Temporarily failed to publish dc_managed_agents_v1 event.
(
    'event4',
    'dc_managed_agents_v1',
    '{"count":4}',
    '2023-01-01 00:00:00+00',
    NULL,
    NULL,
    'publish failed temporarily'
),
-- Permanently failed to publish dc_managed_agents_v1 event.
(
    'event5',
    'dc_managed_agents_v1',
    '{"count":5}',
    '2023-01-01 00:00:00+00',
    NULL,
    '2023-01-01 00:00:02+00',
    'publish failed permanently'
)
