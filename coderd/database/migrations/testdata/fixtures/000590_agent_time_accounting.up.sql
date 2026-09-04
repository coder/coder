-- Covers agent time accounting tables introduced by migration 000590.
-- The chat_message insert exercises live trigger capture into
-- chat_message_agent_time_accounted and agent_time_daily.
INSERT INTO chat_messages (
    chat_id,
    created_at,
    role,
    content,
    content_version,
    visibility,
    runtime_ms
) VALUES (
    'f5610000-0000-4000-8000-000000000007',
    '2024-01-02 00:00:00+00',
    'assistant',
    '[]'::jsonb,
    1,
    'both',
    1234
);

INSERT INTO agent_time_backfill_status (
    organization_id,
    cursor_message_id,
    processed_messages,
    completed_at
) VALUES (
    'f5610000-0000-4000-8000-000000000001',
    0,
    0,
    '2024-01-02 00:00:00+00'
)
ON CONFLICT (organization_id) DO NOTHING;
