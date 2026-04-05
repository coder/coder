-- Update any chats using the requires_action status before removing it.
UPDATE chats SET status = 'pending' WHERE status = 'requires_action';

ALTER TABLE chats DROP COLUMN dynamic_tools;

-- We can't drop values from enums, so we have to create a new one and convert the data.
ALTER TYPE chat_status RENAME TO chat_status_old;
CREATE TYPE chat_status AS ENUM (
    'waiting',
    'pending',
    'running',
    'paused',
    'completed',
    'error'
);
ALTER TABLE chats ALTER COLUMN status DROP DEFAULT;
ALTER TABLE chats ALTER COLUMN status TYPE chat_status USING status::text::chat_status;
ALTER TABLE chats ALTER COLUMN status SET DEFAULT 'waiting';
DROP TYPE chat_status_old;
