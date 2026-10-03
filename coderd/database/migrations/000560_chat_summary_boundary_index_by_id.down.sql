DROP INDEX idx_chat_messages_compressed_summary_boundary;

CREATE INDEX idx_chat_messages_compressed_summary_boundary
	ON chat_messages(chat_id, created_at DESC, id DESC)
	WHERE compressed = TRUE
		AND role = 'system'
		AND visibility IN ('model', 'both');
