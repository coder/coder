INSERT INTO chat_memory_consolidations (
    id,
    organization_id,
    user_id,
    status,
    model,
    memories_before,
    memories_after,
    mutations
)
VALUES (
    '59800000-0000-4000-8000-000000000001',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'succeeded',
    'fixture-model',
    2,
    1,
    '[{"op":"merge","into":"fixture-memory","from":["duplicate-memory"]}]'
);
