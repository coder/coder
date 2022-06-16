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
    ('11111111-1111-1111-1111-111111111111', 'system@coder.com', 'system', '{}', NOW(), NOW(), '{}');
