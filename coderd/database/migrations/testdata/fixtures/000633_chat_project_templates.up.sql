UPDATE chat_projects
SET
    template_ids = ARRAY['6b298946-7a4f-47ac-9158-b03b08740a41']::uuid[],
    default_template_id = '6b298946-7a4f-47ac-9158-b03b08740a41',
    default_template_version_preset_id = '28b42cc0-c4fe-4907-a0fe-e4d20f1e9bfe'
WHERE id = '59400000-0000-4000-8000-000000000001';
