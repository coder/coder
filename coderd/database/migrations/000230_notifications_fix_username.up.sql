UPDATE notification_templates
SET
    actions = REPLACE(actions::text, '@{{.UserName}}', '@{{.UserUsername}}')::jsonb;
