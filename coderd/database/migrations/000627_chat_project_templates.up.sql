ALTER TABLE chat_projects
    ADD COLUMN template_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    ADD COLUMN default_template_id uuid REFERENCES templates(id) ON DELETE SET NULL,
    ADD COLUMN default_template_version_preset_id uuid REFERENCES template_version_presets(id) ON DELETE SET NULL,
    ADD COLUMN default_parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT chat_projects_default_template_in_templates
        CHECK (default_template_id IS NULL OR default_template_id = ANY(template_ids)),
    ADD CONSTRAINT chat_projects_default_preset_or_parameters
        CHECK (default_template_version_preset_id IS NULL OR default_parameters = '{}'::jsonb),
    ADD CONSTRAINT chat_projects_defaults_require_template
        CHECK (default_template_id IS NOT NULL OR (default_template_version_preset_id IS NULL AND default_parameters = '{}'::jsonb));

COMMENT ON COLUMN chat_projects.template_ids IS 'Templates the project''s chats may use. Empty means the chats cannot create or attach workspaces.';
COMMENT ON COLUMN chat_projects.default_template_id IS 'Template recommended to the project''s chats. Must be one of template_ids.';
COMMENT ON COLUMN chat_projects.default_template_version_preset_id IS 'Preset applied when a chat creates a workspace from the default template without choosing one.';
COMMENT ON COLUMN chat_projects.default_parameters IS 'Parameter values, keyed by name, applied beneath the values a chat passes when it creates a workspace from the default template.';
