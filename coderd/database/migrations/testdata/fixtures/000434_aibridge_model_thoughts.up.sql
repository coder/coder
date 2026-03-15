INSERT INTO
    aibridge_model_thoughts (
        id,
        interception_id,
        content,
        metadata,
        created_at
    )
VALUES (
    'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
    'be003e1e-b38f-43bf-847d-928074dd0aa8', -- from 000370_aibridge.up.sql
    'The user is asking about their workspaces. I should use the coder_list_workspaces tool to retrieve this information.',
    '{"source": "commentary"}',
    '2025-09-15 12:45:19.123456+00'
);
