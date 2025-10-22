DO
$$
    DECLARE
        template text;
    BEGIN
        SELECT 'You successfully did {{.thing}}!' INTO template;

        INSERT INTO notification_templates (id, name, title_template, body_template, "group")
        VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'A', template, template, 'Group 1'),
               ('b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12', 'B', template, template, 'Group 1'),
               ('c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13', 'C', template, template, 'Group 2');

        INSERT INTO users(id, email, username, hashed_password, created_at, updated_at, status, rbac_roles, deleted)
        VALUES ('fc1511ef-4fcf-4a3b-98a1-8df64160e35a', 'githubuser@coder.com', 'githubuser', '\x', '2022-11-02 13:05:21.445455+02', '2022-11-02 13:05:21.445455+02', 'active', '{}', false) ON CONFLICT DO NOTHING;

        INSERT INTO notification_messages (id, notification_template_id, user_id, method, created_by, payload)
        VALUES (
                gen_random_uuid(), 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'fc1511ef-4fcf-4a3b-98a1-8df64160e35a', 'smtp'::notification_method, 'test', '{}'
               );
    END
$$;
