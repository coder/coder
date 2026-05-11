-- Fixture for migration 000491: covers the new chat_messages.tool_call_count
-- column. The migration 000422 fixture already created chat
-- 72c0438a-18eb-4688-ab80-e4c6a126ef96 with a single fixture message; this
-- fixture exercises the tool_call_count column on new rows representing
-- a single agent turn (user message + assistant message + tool-result).
INSERT INTO chat_messages (
    chat_id, role, content, content_version, visibility, tool_call_count
) VALUES
    (
        '72c0438a-18eb-4688-ab80-e4c6a126ef96',
        'user',
        '[{"type":"text","text":"hello"}]'::jsonb,
        1,
        'both',
        0
    ),
    (
        '72c0438a-18eb-4688-ab80-e4c6a126ef96',
        'assistant',
        '[
            {"type":"reasoning","text":"thinking"},
            {"type":"tool-call","tool_call_id":"a","tool_name":"bash"},
            {"type":"text","text":"working"},
            {"type":"tool-call","tool_call_id":"b","tool_name":"bash"},
            {"type":"tool-call","tool_call_id":"c","tool_name":"edit"}
        ]'::jsonb,
        1,
        'both',
        3
    ),
    (
        '72c0438a-18eb-4688-ab80-e4c6a126ef96',
        'tool',
        '[{"type":"tool-result","tool_call_id":"a","result":{}}]'::jsonb,
        1,
        'both',
        0
    );
