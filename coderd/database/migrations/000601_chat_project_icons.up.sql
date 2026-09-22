-- Icons follow the template and organization convention: a URL, usually an
-- emoji image under /emojis, rendered by the frontend.
ALTER TABLE chat_projects ADD COLUMN icon text NOT NULL DEFAULT '';
ALTER TABLE chat_projects ADD CONSTRAINT chat_projects_icon_length CHECK (length(icon) <= 256);
COMMENT ON COLUMN chat_projects.icon IS 'Optional icon URL shown next to the project name.';
