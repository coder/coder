-- Insert a chat_run for the test chat created in 000422.
-- The trigger auto-assigns `number` by incrementing
-- `last_run_number`, so we reset the counter first to
-- ensure a deterministic fixture.
UPDATE chats SET last_run_number = 0
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

INSERT INTO chat_runs (id, chat_id, created_at)
VALUES (
    '00000000-0000-0000-0000-000000000201',
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    '2024-01-01 00:00:00+00'
);

-- Insert a completed chat_run_step for the run above.
INSERT INTO chat_run_steps (id, chat_run_id, chat_id, started_at, heartbeat_at, completed_at)
VALUES (
    '00000000-0000-0000-0000-000000000301',
    '00000000-0000-0000-0000-000000000201',
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:01+00'
);
