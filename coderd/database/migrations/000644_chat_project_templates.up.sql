CREATE TABLE chat_project_templates (
    project_id uuid NOT NULL REFERENCES chat_projects(id) ON DELETE CASCADE,
    template_id uuid NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, template_id)
);

COMMENT ON TABLE chat_project_templates IS 'Templates attached to chat projects. An attachment never grants access to its template; every reader is checked against the template itself.';

ALTER TABLE chat_projects
    ADD COLUMN limit_templates boolean NOT NULL DEFAULT false,
    ADD COLUMN default_template_id uuid REFERENCES templates(id) ON DELETE SET NULL,
    ADD COLUMN default_template_version_preset_id uuid REFERENCES template_version_presets(id) ON DELETE SET NULL,
    ADD COLUMN default_parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT chat_projects_default_preset_or_parameters
        CHECK (default_template_version_preset_id IS NULL OR default_parameters = '{}'::jsonb);

COMMENT ON COLUMN chat_projects.limit_templates IS 'When true, the project''s chats can only use the templates in chat_project_templates, and none when it has no rows. When false, they can use every template their owner can use.';
COMMENT ON COLUMN chat_projects.default_template_id IS 'Attached template recommended to the project''s chats. Ignored while the template is not attached.';
COMMENT ON COLUMN chat_projects.default_template_version_preset_id IS 'Preset applied when a chat creates a workspace from the default template without choosing one.';
COMMENT ON COLUMN chat_projects.default_parameters IS 'Parameter values, keyed by name, applied beneath the values a chat passes when it creates a workspace from the default template.';
