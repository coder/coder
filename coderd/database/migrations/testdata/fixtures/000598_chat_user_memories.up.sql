INSERT INTO chat_user_memories (
    id,
    organization_id,
    user_id,
    name,
    description,
    body
)
VALUES (
    '59600000-0000-4000-8000-000000000001',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'fixture-memory',
    'Fixture user memory.',
    'This memory exists for migration fixtures.'
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
    '59600000-0000-4000-8000-000000000002',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    id,
    'Memory Cursor Fixture Chat',
    'waiting',
    'api'
FROM chat_model_configs
LIMIT 1;

INSERT INTO chat_memory_cursors (chat_id, history_version)
VALUES ('59600000-0000-4000-8000-000000000002', 1);
