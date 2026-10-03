-- Migration 424 adds chats.last_error as text. Seed one existing fixture
-- chat with a legacy plain-text error so migration 485 has a non-null row
-- to backfill, and add a second chat that leaves last_error NULL so the
-- migration fixture can assert both branches of the CASE expression.
UPDATE chats
SET last_error = 'Legacy provider failure'
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

INSERT INTO chats (
    id,
    owner_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at
)
SELECT
    '5a4ac6a3-9dc5-440f-ae6b-5805e477bc59',
    owner_id,
    last_model_config_id,
    'Fixture Chat With Null Error',
    'waiting',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM chats
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';
