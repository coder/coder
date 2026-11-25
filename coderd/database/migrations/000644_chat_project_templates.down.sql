ALTER TABLE chat_projects
    DROP COLUMN default_parameters,
    DROP COLUMN default_template_version_preset_id,
    DROP COLUMN default_template_id,
    DROP COLUMN limit_templates;

DROP TABLE chat_project_templates;
