INSERT INTO notification_templates (id, name, title_template, body_template, "group")
VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'A', 'title', 'body', 'Group 1') ON CONFLICT DO NOTHING;

INSERT INTO notification_preferences (user_id, notification_template_id, disabled, created_at, updated_at)
VALUES ('a0061a8e-7db7-4585-838c-3116a003dd21', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', FALSE, '2024-07-15 10:30:00+00', '2024-07-15 10:30:00+00');
