-- A nested instruction file chatd discovered from a tool-touched directory
-- rather than copying from the agent snapshot, attached to the first chat
-- like the other chat_context_resources fixtures.
INSERT INTO chat_context_resources (
    chat_id,
    source,
    body_kind,
    body,
    content_hash,
    size_bytes,
    status,
    error,
    source_path,
    discovered
)
SELECT
    c.id,
    '/home/coder/workspace/site/AGENTS.md',
    'instruction_file'::workspace_agent_context_body_kind,
    '{"content":"c2l0ZSBydWxlcw=="}'::jsonb,
    decode('6666666666666666666666666666666666666666666666666666666666666666', 'hex'),
    10::bigint,
    'ok'::workspace_agent_context_resource_status,
    '',
    '/home/coder/workspace/site',
    true
FROM (
    SELECT id FROM chats ORDER BY created_at, id LIMIT 1
) AS c;
