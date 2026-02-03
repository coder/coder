INSERT INTO chats VALUES (
    '5c5c5a05-3b11-4f85-8e06-5b7f8c2c67e1', -- id
    '2024-11-02 13:10:00.000000+02',        -- created_at
    '2024-11-02 13:10:00.000000+02',        -- updated_at
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization_id
    '30095c71-380b-457a-8995-97b8ee6e5307', -- owner_id
    NULL,                                   -- workspace_id
    'Test Chat 1',                          -- title
    'openai',                               -- provider
    'gpt-4o-mini',                          -- model
    '{}'::JSONB                             -- metadata
) ON CONFLICT DO NOTHING;

INSERT INTO chat_messages (
    chat_id,
    created_at,
    role,
    content
) VALUES (
    '5c5c5a05-3b11-4f85-8e06-5b7f8c2c67e1', -- chat_id
    '2024-11-02 13:10:01.000000+02',        -- created_at
    'user',                                 -- role
    '{"type":"message","message":{"role":"user","content":"hello","parts":[{"type":"text","text":"hello"}]}}'::JSONB -- content
) ON CONFLICT DO NOTHING;
