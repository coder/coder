UPDATE notification_templates
SET
    actions = REPLACE(actions::text, '@{{.UserUsername}}', '@{{.UserName}}')::jsonb;
