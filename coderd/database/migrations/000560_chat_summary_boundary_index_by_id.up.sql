-- The predicate required role = 'system', but compaction writes its summary
-- with the user role, so this index has never matched a row. Rebuild it to
-- match GetChatMessagesForPromptByChatID's boundary lookup, which orders by id.
DROP INDEX idx_chat_messages_compressed_summary_boundary;

CREATE INDEX idx_chat_messages_compressed_summary_boundary
	ON chat_messages(chat_id, id DESC)
	WHERE compressed = TRUE
		AND deleted = false
		AND visibility = 'model';
