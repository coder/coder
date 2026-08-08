ALTER TABLE chat_messages DROP COLUMN content_version;

DROP INDEX idx_chat_messages_compressed_summary_boundary;

ALTER TABLE chat_messages
	ALTER COLUMN role TYPE text
		USING (role::text);

CREATE INDEX idx_chat_messages_compressed_summary_boundary
	ON chat_messages(chat_id, created_at DESC, id DESC)
	WHERE compressed = TRUE
		AND role = 'system'
		AND visibility IN ('model', 'both');

DROP TYPE chat_message_role;
