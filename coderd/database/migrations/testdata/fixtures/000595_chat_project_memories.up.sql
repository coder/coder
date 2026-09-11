INSERT INTO chat_project_memories (
    id,
    project_id,
    organization_id,
    type,
    name,
    description,
    body,
    created_by
)
VALUES (
    '59500000-0000-4000-8000-000000000001',
    '59400000-0000-4000-8000-000000000001',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    'project',
    'fixture-memory',
    'Fixture project memory.',
    'This memory exists for migration fixtures.',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94'
);

INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    status,
    client_type
)
SELECT
    '59500000-0000-4000-8000-000000000002',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    id,
    'Memory Cursor Fixture Chat',
    'waiting',
    'api'
FROM chat_model_configs
LIMIT 1;

INSERT INTO chat_project_memory_cursors (chat_id, history_version)
VALUES ('59500000-0000-4000-8000-000000000002', 1);
