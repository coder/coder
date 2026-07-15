-- Ledger rows across the lifecycle: a reserved intent, a running
-- claim with a recorded process, an exited row whose result was
-- committed, and an unconfirmed cancellation. Attach to the first
-- assistant message so migration tests see non-empty rows without
-- hard-coding fixture IDs.
INSERT INTO chat_tool_call_executions (
    id,
    chat_id,
    assistant_message_id,
    tool_call_id,
    status,
    input_sha256,
    command,
    background,
    timeout_secs,
    claim_epoch,
    claimed_at,
    process_id,
    cancel_signal_sent_at,
    result_committed_at,
    created_at,
    started_at,
    updated_at
)
SELECT
    v.id,
    m.chat_id,
    m.id,
    v.tool_call_id,
    v.status,
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    v.command,
    false,
    v.timeout_secs,
    v.claim_epoch,
    v.claimed_at,
    v.process_id,
    v.cancel_signal_sent_at,
    v.result_committed_at,
    '2026-01-01 00:00:00+00'::timestamptz,
    v.started_at,
    '2026-01-01 00:00:02+00'::timestamptz
FROM (
    SELECT id, chat_id FROM chat_messages
    WHERE role = 'assistant'
    ORDER BY id
    LIMIT 1
) AS m
CROSS JOIN (
    VALUES
        (
            '10000000-0000-0000-0000-000000000001'::uuid,
            'toolu_fixture_reserved',
            'reserved'::chat_tool_call_execution_status,
            '',
            0::bigint,
            0::bigint,
            NULL::timestamptz,
            NULL::text,
            NULL::timestamptz,
            NULL::timestamptz,
            NULL::timestamptz
        ),
        (
            '10000000-0000-0000-0000-000000000002'::uuid,
            'toolu_fixture_running',
            'running'::chat_tool_call_execution_status,
            'sleep 600',
            600::bigint,
            1::bigint,
            '2026-01-01 00:00:01+00'::timestamptz,
            'proc-fixture-1',
            NULL::timestamptz,
            NULL::timestamptz,
            '2026-01-01 00:00:01+00'::timestamptz
        ),
        (
            '10000000-0000-0000-0000-000000000003'::uuid,
            'toolu_fixture_exited',
            'exited'::chat_tool_call_execution_status,
            'echo done',
            10::bigint,
            1::bigint,
            '2026-01-01 00:00:01+00'::timestamptz,
            'proc-fixture-2',
            NULL::timestamptz,
            '2026-01-01 00:00:02+00'::timestamptz,
            '2026-01-01 00:00:01+00'::timestamptz
        ),
        (
            '10000000-0000-0000-0000-000000000004'::uuid,
            'toolu_fixture_cancel_requested',
            'cancel_requested'::chat_tool_call_execution_status,
            'sleep 600',
            600::bigint,
            1::bigint,
            '2026-01-01 00:00:01+00'::timestamptz,
            'proc-fixture-3',
            '2026-01-01 00:00:02+00'::timestamptz,
            '2026-01-01 00:00:02+00'::timestamptz,
            '2026-01-01 00:00:01+00'::timestamptz
        )
) AS v(id, tool_call_id, status, command, timeout_secs, claim_epoch, claimed_at, process_id, cancel_signal_sent_at, result_committed_at, started_at);
