ALTER TABLE chat_messages
	ADD COLUMN environment_variables jsonb;

COMMENT ON COLUMN chat_messages.environment_variables IS
	'Turn-scoped environment variables for workspace command execution. Not included in model prompts or API responses.';

ALTER TABLE chat_queued_messages
	ADD COLUMN environment_variables jsonb;

COMMENT ON COLUMN chat_queued_messages.environment_variables IS
	'Turn-scoped environment variables retained until the queued message is promoted.';
