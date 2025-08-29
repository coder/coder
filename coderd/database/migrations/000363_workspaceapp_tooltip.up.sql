ALTER TABLE workspace_apps
	ADD COLUMN IF NOT EXISTS tooltip VARCHAR(512) NOT NULL DEFAULT '';

COMMENT ON COLUMN workspace_apps.tooltip IS 'Markdown-supported text that is displayed when hovering over app icons in the workspace dashboard (max 512 characters).';
