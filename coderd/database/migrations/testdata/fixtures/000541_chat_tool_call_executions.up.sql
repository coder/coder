-- Execution records in both lifecycle states: reserved without a
-- process handle, and started with a recorded handle. Attach to the
-- first existing chat so migration tests see non-empty rows without
-- hard-coding a chat ID.
INSERT INTO chat_tool_call_executions (
    chat_id,
    tool_call_id,
    process_id,
    command,
    background,
    timeout_secs,
    created_at,
    started_at
)
SELECT
    c.id,
    v.tool_call_id,
    v.process_id,
    v.command,
    v.background,
    v.timeout_secs,
    '2026-01-01 00:00:00+00'::timestamptz,
    v.started_at
FROM (
    SELECT id FROM chats ORDER BY created_at, id LIMIT 1
) AS c
CROSS JOIN (
    VALUES
        (
            'toolu_fixture_reserved',
            NULL,
            'echo reserved',
            false,
            10::bigint,
            NULL::timestamptz
        ),
        (
            'toolu_fixture_started',
            'proc-fixture-1',
            'sleep 600',
            false,
            600::bigint,
            '2026-01-01 00:00:01+00'::timestamptz
        )
) AS v(tool_call_id, process_id, command, background, timeout_secs, started_at);
