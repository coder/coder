ALTER TABLE workspace_apps
	ADD COLUMN IF NOT EXISTS tooltip VARCHAR(2048) NOT NULL DEFAULT '';

COMMENT ON COLUMN workspace_apps.tooltip IS 'Markdown text that is displayed when hovering over workspace apps.';
