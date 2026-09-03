ALTER TABLE chat_messages
	DROP COLUMN api_key_id;

ALTER TABLE chat_queued_messages
	DROP COLUMN api_key_id;
