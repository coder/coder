ALTER TABLE chat_projects
	ADD COLUMN icon text NOT NULL DEFAULT '',
	ADD CONSTRAINT chat_projects_icon_length CHECK (length(icon) <= 256);

COMMENT ON COLUMN chat_projects.icon IS 'URL or path to an icon representing the chat project (max 256 characters).';
