CREATE TYPE chat_busy_behavior AS ENUM ('queue', 'steer');

ALTER TABLE chat_queued_messages
    ADD COLUMN busy_behavior chat_busy_behavior NOT NULL DEFAULT 'queue';

COMMENT ON COLUMN chat_queued_messages.busy_behavior IS 'queue: delivered at turn end. steer: delivered before the next model call.';
