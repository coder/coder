-- Migration 424 adds chats.last_error as text. Seed the existing fixture
-- chat with a legacy plain-text error so migration 474 has a non-null row
-- to backfill.
UPDATE chats
SET last_error = 'Legacy provider failure'
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';
