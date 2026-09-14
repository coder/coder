ALTER TABLE chat_messages
	ADD COLUMN api_key_id text;

ALTER TABLE chat_queued_messages
	ADD COLUMN api_key_id text;
