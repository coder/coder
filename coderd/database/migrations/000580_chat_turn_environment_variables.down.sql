ALTER TABLE chat_queued_messages
	DROP COLUMN environment_variables;

ALTER TABLE chat_messages
	DROP COLUMN environment_variables;
