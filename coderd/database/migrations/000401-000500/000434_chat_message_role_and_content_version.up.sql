-- Add chat_message_role enum.
CREATE TYPE chat_message_role AS ENUM (
	'system',
	'user',
	'assistant',
	'tool'
);

-- Drop the partial index that references role as text before
-- converting the column type.
DROP INDEX idx_chat_messages_compressed_summary_boundary;

-- Convert role column from text to enum.
ALTER TABLE chat_messages
	ALTER COLUMN role TYPE chat_message_role
		USING (role::chat_message_role);

-- Recreate the partial index with enum-typed comparison.
CREATE INDEX idx_chat_messages_compressed_summary_boundary
	ON chat_messages(chat_id, created_at DESC, id DESC)
	WHERE compressed = TRUE
		AND role = 'system'
		AND visibility IN ('model', 'both');

-- Add content_version column. Default 0 backfills existing rows.
-- The default is then dropped so future inserts must specify the
-- version explicitly.
ALTER TABLE chat_messages
	ADD COLUMN content_version smallint NOT NULL DEFAULT 0;

ALTER TABLE chat_messages
	ALTER COLUMN content_version DROP DEFAULT;
