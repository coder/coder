CREATE TYPE user_kind AS ENUM ('human', 'ai_agent');

ALTER TABLE users
	ADD COLUMN kind user_kind NOT NULL DEFAULT 'human';

ALTER TABLE users
	DROP CONSTRAINT users_service_account_login_type,
	DROP CONSTRAINT users_email_not_empty;

-- Service accounts and AI agents cannot use interactive login methods.
ALTER TABLE users
	ADD CONSTRAINT users_service_account_login_type CHECK (
		(is_service_account = false AND kind != 'ai_agent') OR login_type = 'none'
	),
	ADD CONSTRAINT users_email_not_empty CHECK (
		(email = '') = (is_service_account = true OR kind = 'ai_agent')
	);

CREATE TYPE ai_agent_origin AS ENUM ('chat', 'workspace');

CREATE TABLE ai_agents (
	user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	origin_type ai_agent_origin NOT NULL,
	origin_id uuid NOT NULL,
	created_at timestamp with time zone NOT NULL DEFAULT now(),
	deleted boolean NOT NULL DEFAULT false
);

CREATE INDEX idx_ai_agents_owner ON ai_agents (owner_user_id);
CREATE UNIQUE INDEX idx_ai_agents_origin ON ai_agents (origin_type, origin_id)
	WHERE NOT deleted;

ALTER TABLE audit_logs
	ADD COLUMN on_behalf_of_user_id uuid;

CREATE INDEX idx_audit_logs_on_behalf_of_user_id
	ON audit_logs (on_behalf_of_user_id);
