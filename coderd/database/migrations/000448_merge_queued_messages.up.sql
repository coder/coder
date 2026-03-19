-- Merge chat_queued_messages into chat_messages.
-- Adds a "queued" boolean column so queued messages live in the same table
-- as regular chat messages.

ALTER TABLE chat_messages ADD COLUMN queued boolean NOT NULL DEFAULT false;

-- Migrate existing queued messages into chat_messages.
INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, queued)
SELECT chat_id, 'user', content, 1, 'both', true
FROM chat_queued_messages
ORDER BY id ASC;

DROP TABLE chat_queued_messages;
