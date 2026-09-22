ALTER TABLE chat_messages ADD COLUMN aibridge_interception_id uuid;

COMMENT ON COLUMN chat_messages.aibridge_interception_id IS
    'AI Gateway interception (aibridge_interceptions.id) that produced this message. No foreign key because gateway retention is independent.';
