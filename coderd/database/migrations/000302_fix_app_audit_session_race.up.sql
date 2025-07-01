-- Add column with default to fix existing rows.
ALTER TABLE workspace_app_audit_sessions
	ADD COLUMN id UUID PRIMARY KEY DEFAULT gen_random_uuid();
ALTER TABLE workspace_app_audit_sessions
	ALTER COLUMN id DROP DEFAULT;
