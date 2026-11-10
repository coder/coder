-- display_name labels a project skill's action chip. An empty value falls
-- back to the skill name.
ALTER TABLE skills
    ADD COLUMN display_name text NOT NULL DEFAULT '',
    ADD CONSTRAINT skills_display_name_project_only CHECK (display_name = '' OR project_id IS NOT NULL),
    ADD CONSTRAINT skills_display_name_length CHECK (char_length(display_name) <= 64);
