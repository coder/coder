INSERT INTO chat_project_templates (project_id, template_id, created_by)
VALUES (
    '59400000-0000-4000-8000-000000000001',
    '6b298946-7a4f-47ac-9158-b03b08740a41',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94'
);

UPDATE chat_projects
SET
    limit_templates = true,
    default_template_id = '6b298946-7a4f-47ac-9158-b03b08740a41',
    default_template_version_preset_id = '28b42cc0-c4fe-4907-a0fe-e4d20f1e9bfe'
WHERE id = '59400000-0000-4000-8000-000000000001';
