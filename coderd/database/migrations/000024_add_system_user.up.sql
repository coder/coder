INSERT INTO
    users (
        id,
        email,
        username,
        hashed_password,
        created_at,
        updated_at,
        rbac_roles
    )
VALUES
    ('c0de2b07-0000-4000-A000-000000000000', 'system@coder.com', 'system', '', NOW(), NOW(), '{}');
