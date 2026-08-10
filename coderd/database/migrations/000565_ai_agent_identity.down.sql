DROP INDEX idx_audit_logs_on_behalf_of_user_id;
ALTER TABLE audit_logs DROP COLUMN on_behalf_of_user_id;

DROP INDEX idx_ai_agents_origin;
DROP INDEX idx_ai_agents_owner;
DROP TABLE ai_agents;
DROP TYPE ai_agent_origin;

ALTER TABLE users
	DROP CONSTRAINT users_service_account_login_type,
	DROP CONSTRAINT users_email_not_empty;

-- Preserve rolled-back AI agent users as non-interactive service accounts.
UPDATE users
SET is_service_account = true
WHERE kind = 'ai_agent';

ALTER TABLE users DROP COLUMN kind;

ALTER TABLE users
	ADD CONSTRAINT users_service_account_login_type CHECK (
		is_service_account = false OR login_type = 'none'
	),
	ADD CONSTRAINT users_email_not_empty CHECK (
		(is_service_account = true) = (email = '')
	);

DROP TYPE user_kind;
