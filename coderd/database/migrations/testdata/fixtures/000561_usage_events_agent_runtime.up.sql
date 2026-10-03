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
-- Unpublished hb_agent_runtime_v1 event.
(
    'hb_agent_runtime_v1:2023-06-01_00:00:00',
    'hb_agent_runtime_v1',
    '{"runtime_ms":3600000}',
    '2023-06-01 00:00:00+00',
    NULL,
    NULL,
    NULL
);
