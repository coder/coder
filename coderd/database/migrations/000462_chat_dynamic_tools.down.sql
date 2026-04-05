-- First update any rows using the value we're about to remove.
-- The column type is still the original chat_status at this point.
UPDATE chats SET status = 'pending' WHERE status = 'requires_action';

-- Drop the column (this is independent of the enum).
ALTER TABLE chats DROP COLUMN IF EXISTS dynamic_tools;

-- Now recreate the enum without requires_action.
-- We must use the rename-create-cast-drop pattern.
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
