INSERT INTO chat_artifacts (
    id,
    organization_id,
    owner_id,
    project_id,
    title
)
VALUES (
    '59600000-0000-4000-8000-000000000001',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    '59400000-0000-4000-8000-000000000001',
    'Fixture artifact'
);

INSERT INTO chat_artifact_versions (
    artifact_id,
    version,
    label,
    name,
    mimetype,
    data,
    created_by,
    chat_id
)
SELECT
    '59600000-0000-4000-8000-000000000001',
    1,
    'first',
    'fixture.md',
    'text/markdown',
    '\x232046697874757265'::bytea,
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    (SELECT id FROM chats ORDER BY created_at DESC, id LIMIT 1);
