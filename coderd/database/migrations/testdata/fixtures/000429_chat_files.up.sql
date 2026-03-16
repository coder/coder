INSERT INTO chat_files (id, owner_id, organization_id, created_at, name, mimetype, data)
SELECT
    '00000000-0000-0000-0000-000000000099',
    u.id,
    om.organization_id,
    '2024-01-01 00:00:00+00',
    'test.png',
    'image/png',
    E'\\x89504E47'
FROM users u
JOIN organization_members om ON om.user_id = u.id
ORDER BY u.created_at, u.id
LIMIT 1;
