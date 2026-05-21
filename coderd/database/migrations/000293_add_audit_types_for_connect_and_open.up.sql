-- Add new audit types for connect and open actions.
ALTER TYPE audit_action
	ADD VALUE IF NOT EXISTS 'connect';
ALTER TYPE audit_action
	ADD VALUE IF NOT EXISTS 'disconnect';
ALTER TYPE resource_type
	ADD VALUE IF NOT EXISTS 'workspace_agent';
ALTER TYPE audit_action
	ADD VALUE IF NOT EXISTS 'open';
ALTER TYPE audit_action
	ADD VALUE IF NOT EXISTS 'close';
ALTER TYPE resource_type
	ADD VALUE IF NOT EXISTS 'workspace_app';
