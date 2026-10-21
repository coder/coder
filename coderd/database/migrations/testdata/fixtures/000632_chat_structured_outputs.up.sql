INSERT INTO chat_structured_outputs (message_id, chat_id, status, request, value)
SELECT id, chat_id, 'succeeded', '{"name":"report","schema":{"type":"object"}}', '{"ok":true}'
FROM chat_messages
WHERE chat_id = '72c0438a-18eb-4688-ab80-e4c6a126ef96'
ORDER BY id
LIMIT 1;
